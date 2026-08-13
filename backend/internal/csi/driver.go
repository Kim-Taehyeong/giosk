package csi

import (
	"context"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DriverName은 CSIDriver 오브젝트와 StorageClass provisioner 에 쓰는 이름이다.
const DriverName = "csi.giosk.io"

// Version은 Identity 응답에 싣는 드라이버 버전.
const Version = "0.1.0"

// TopologyKey는 볼륨이 어느 노드에 묶였는지 나타내는 토폴로지 키다.
// 이미지가 노드 로컬 디스크에 있으므로 볼륨은 그 노드를 벗어날 수 없다. 스케줄러는
// PV 의 nodeAffinity 로 이 제약을 보고 파드를 같은 노드에 배치한다.
const TopologyKey = "topology." + DriverName + "/node"

// Driver는 노드 플러그인과 컨트롤러 플러그인을 같은 바이너리로 제공한다.
// 실행 모드는 NodeID 유무로 갈린다(노드 플러그인만 NodeID 를 받는다).
type Driver struct {
	csi.UnimplementedIdentityServer
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer

	NodeID string      // 노드 플러그인일 때만 설정. 비면 컨트롤러 모드.
	Store  *ImageStore // 노드 플러그인에서만 사용
}

// ── Identity ────────────────────────────────────────────────────────

func (d *Driver) GetPluginInfo(context.Context, *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{Name: DriverName, VendorVersion: Version}, nil
}

func (d *Driver) Probe(context.Context, *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	return &csi.ProbeResponse{}, nil
}

func (d *Driver) GetPluginCapabilities(context.Context, *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{Capabilities: []*csi.PluginCapability{
		{Type: &csi.PluginCapability_Service_{Service: &csi.PluginCapability_Service{
			Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
		}}},
		// 볼륨이 특정 노드에 묶인다는 것을 스케줄러에 알린다.
		{Type: &csi.PluginCapability_Service_{Service: &csi.PluginCapability_Service{
			Type: csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS,
		}}},
	}}, nil
}

// ── Controller ──────────────────────────────────────────────────────

func (d *Driver) ControllerGetCapabilities(context.Context, *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	caps := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
	}
	out := make([]*csi.ControllerServiceCapability, 0, len(caps))
	for _, c := range caps {
		out = append(out, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{Rpc: &csi.ControllerServiceCapability_RPC{Type: c}},
		})
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: out}, nil
}

// CreateVolume은 볼륨을 "등록"만 한다. 실제 이미지는 NodePublishVolume 이 만든다.
//
// 이미지가 노드 로컬이라 컨트롤러는 디스크에 손댈 수 없다. 대신 스케줄러가 고른 노드를
// accessible_topology 로 돌려주면 external-provisioner 가 그 값으로 PV 의 nodeAffinity 를
// 세운다. StorageClass 는 WaitForFirstConsumer 여야 하며, 그래야 파드가 스케줄된 뒤에
// 이 호출이 오고 노드가 정해진 상태가 된다.
func (d *Driver) CreateVolume(_ context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	name := req.GetName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "볼륨 이름이 비어 있습니다")
	}
	size := req.GetCapacityRange().GetRequiredBytes()
	if size <= 0 {
		return nil, status.Error(codes.InvalidArgument, "볼륨 크기가 지정되지 않았습니다")
	}
	node := selectedNode(req.GetAccessibilityRequirements())
	if node == "" {
		// WaitForFirstConsumer 가 아니면 여기로 온다. 노드를 모르면 이미지를 놓을 곳도 정할 수 없다.
		return nil, status.Error(codes.InvalidArgument,
			"노드 토폴로지가 없습니다. StorageClass 를 volumeBindingMode: WaitForFirstConsumer 로 두어야 합니다")
	}
	// 노드가 이미지를 만들려면 크기를 알아야 하는데 NodePublishVolume 요청에는 용량이 없다.
	// VolumeContext 에 실어 보내면 그대로 노드까지 전달된다.
	ctx := map[string]string{ParamSize: strconv.FormatInt(size, 10)}
	for k, v := range req.GetParameters() {
		ctx[k] = v
	}
	return &csi.CreateVolumeResponse{Volume: &csi.Volume{
		VolumeId:      name,
		CapacityBytes: size,
		VolumeContext: ctx,
		AccessibleTopology: []*csi.Topology{
			{Segments: map[string]string{TopologyKey: node}},
		},
	}}, nil
}

// selectedNode는 스케줄러가 고른 노드를 토폴로지 요구사항에서 꺼낸다.
// preferred 가 먼저고, 없으면 requisite 의 첫 항목을 쓴다.
func selectedNode(t *csi.TopologyRequirement) string {
	if t == nil {
		return ""
	}
	for _, seg := range t.GetPreferred() {
		if n := seg.GetSegments()[TopologyKey]; n != "" {
			return n
		}
	}
	for _, seg := range t.GetRequisite() {
		if n := seg.GetSegments()[TopologyKey]; n != "" {
			return n
		}
	}
	return ""
}

// DeleteVolume은 컨트롤러에서는 성공만 돌려준다. 이미지는 노드 로컬이라 여기서 지울 수 없고,
// 각 노드 플러그인의 고아 정리가 PV 가 사라진 이미지를 회수한다.
func (d *Driver) DeleteVolume(context.Context, *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerExpandVolume은 확장 요청을 받아들이고 실제 확장은 노드가 한다(NodeExpandVolume).
func (d *Driver) ControllerExpandVolume(_ context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         req.GetCapacityRange().GetRequiredBytes(),
		NodeExpansionRequired: true,
	}, nil
}

func (d *Driver) ValidateVolumeCapabilities(_ context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{VolumeCapabilities: req.GetVolumeCapabilities()},
	}, nil
}

// ── Node ────────────────────────────────────────────────────────────

func (d *Driver) NodeGetInfo(context.Context, *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId:             d.NodeID,
		AccessibleTopology: &csi.Topology{Segments: map[string]string{TopologyKey: d.NodeID}},
	}, nil
}

func (d *Driver) NodeGetCapabilities(context.Context, *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{Capabilities: []*csi.NodeServiceCapability{
		{Type: &csi.NodeServiceCapability_Rpc{Rpc: &csi.NodeServiceCapability_RPC{
			Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
		}}},
	}}, nil
}

// NodePublishVolume은 이미지를 준비해 마운트하고 kubelet 이 준 대상 경로에 bind 한다.
// 멱등해야 한다. kubelet 은 같은 호출을 여러 번 보낼 수 있다.
func (d *Driver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	volID := req.GetVolumeId()
	target := req.GetTargetPath()
	if volID == "" || target == "" {
		return nil, status.Error(codes.InvalidArgument, "볼륨 ID 또는 대상 경로가 비어 있습니다")
	}
	// NFS 위 FUSE 볼륨은 크기 개념이 없다(원격 저장소를 그대로 보여 준다). 서버·경로가
	// 있으면 그 경로로 가고, 없으면 로컬 이미지 볼륨이다.
	if spec, ok := nfsFuseFrom(req.GetVolumeContext(), req.GetReadonly()); ok {
		if err := d.mountNFSFuse(ctx, volID, target, spec); err != nil {
			return nil, status.Errorf(codes.Internal, "NFS FUSE 마운트 실패: %v", err)
		}
		return &csi.NodePublishVolumeResponse{}, nil
	}

	size := volumeSize(req.GetVolumeContext())
	if size <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "볼륨 %s 의 크기를 알 수 없습니다", volID)
	}
	if err := d.Store.EnsureImage(ctx, volID, size); err != nil {
		return nil, status.Errorf(codes.Internal, "이미지 준비 실패: %v", err)
	}
	// 소유권은 여기서 손대지 않는다. CSIDriver fsGroupPolicy=File 이라 kubelet 이
	// 파드의 fsGroup(세션 UID)으로 맞춰 준다.
	src, err := d.Store.Mount(ctx, volID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "이미지 마운트 실패: %v", err)
	}
	if err := os.MkdirAll(target, 0o750); err != nil {
		return nil, status.Errorf(codes.Internal, "대상 경로 생성 실패: %v", err)
	}
	mounted, err := IsMountPoint(target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "대상 경로 확인 실패: %v", err)
	}
	if !mounted {
		opts := "bind"
		if req.GetReadonly() {
			opts += ",ro"
		}
		if _, err := run(ctx, "mount", "-o", opts, src, target); err != nil {
			return nil, status.Errorf(codes.Internal, "bind 실패: %v", err)
		}
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume은 대상 경로의 bind 를 푼다. 이미지 마운트 자체는 남긴다.
// 같은 노드의 다른 파드가 쓰고 있을 수 있고, 어차피 재시작 때 다시 쓴다.
func (d *Driver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	target := req.GetTargetPath()
	if target == "" {
		return nil, status.Error(codes.InvalidArgument, "대상 경로가 비어 있습니다")
	}
	// 이 요청에는 볼륨 속성이 없어서 타입을 알 수 없다. NFS 스테이지 경로가 있으면
	// FUSE 볼륨이다(해제 방식이 다르다. FUSE 는 fusermount 로 데몬까지 정리해야 한다).
	if st, err := os.Stat(d.Store.nfsStagePath(req.GetVolumeId())); err == nil && st.IsDir() {
		if err := d.unmountNFSFuse(ctx, req.GetVolumeId(), target); err != nil {
			return nil, status.Errorf(codes.Internal, "NFS FUSE 해제 실패: %v", err)
		}
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}
	mounted, err := IsMountPoint(target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "대상 경로 확인 실패: %v", err)
	}
	if mounted {
		if _, err := run(ctx, "umount", target); err != nil {
			return nil, status.Errorf(codes.Internal, "bind 해제 실패: %v", err)
		}
	}
	if err := os.RemoveAll(target); err != nil {
		return nil, status.Errorf(codes.Internal, "대상 경로 정리 실패: %v", err)
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeExpandVolume은 이미지와 파일시스템을 늘린다(마운트 중 가능).
func (d *Driver) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	size := req.GetCapacityRange().GetRequiredBytes()
	if size <= 0 {
		return nil, status.Error(codes.InvalidArgument, "확장 크기가 지정되지 않았습니다")
	}
	if err := d.Store.EnsureImage(ctx, req.GetVolumeId(), size); err != nil {
		return nil, status.Errorf(codes.Internal, "확장 실패: %v", err)
	}
	return &csi.NodeExpandVolumeResponse{CapacityBytes: size}, nil
}

// ── 서버 기동 ───────────────────────────────────────────────────────

// Serve는 유닉스 소켓에서 gRPC 서버를 띄운다(기존 소켓 파일은 정리한다).
func Serve(endpoint string, d *Driver) error {
	path := strings.TrimPrefix(endpoint, "unix://")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	lis, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	s := grpc.NewServer(grpc.UnaryInterceptor(logInterceptor))
	csi.RegisterIdentityServer(s, d)
	csi.RegisterControllerServer(s, d)
	if d.NodeID != "" {
		csi.RegisterNodeServer(s, d)
	}
	log.Printf("%s 기동 (endpoint=%s node=%q)", DriverName, endpoint, d.NodeID)
	return s.Serve(lis)
}

// logInterceptor는 실패한 호출만 로그로 남긴다. 성공 호출은 kubelet 이 초당 여러 번
// 보내므로 전부 찍으면 로그가 쓸모없어진다.
func logInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
	resp, err := h(ctx, req)
	if err != nil {
		log.Printf("%s 실패: %v", info.FullMethod, err)
	}
	return resp, err
}
