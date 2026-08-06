package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// 세션 노출 모드. nodeport(기본) 또는 loadbalancer(MetalLB) 둘 뿐이다.
const (
	ExposeLoadBalancer = "loadbalancer"
	ExposeNodePort     = "nodeport"
)

// SvcSpec은 세션 웹 노출 Service 입력.
type SvcSpec struct {
	Namespace string
	Name      string    // = instance_id (pod 와 동일; selector giosk.io/session)
	Ports     []SvcPort // 노출 포트(웹 채널 + sshd 22). 첫 항목이 primary(외부 노출 좌표).
	Mode      string    // loadbalancer | nodeport
	Internal  bool      // 게이트웨이 라우팅용: 외부 노출 대신 ClusterIP Service 를 만들어 인클러스터 DNS 로 도달.
}

// SvcPort는 세션 Service 가 노출하는 포트 1개.
type SvcPort struct {
	Name string // web | jupyter | ssh
	Port int
}

// SvcAccess는 생성된 Service 의 외부 접속 좌표.
type SvcAccess struct {
	Mode     string
	LBIP     string // loadbalancer 할당 IP(없으면 대기중)
	NodePort int    // nodeport 할당 포트(primary=첫 포트=웹 채널)
	// sshd 사이드카 포트("ssh")의 NodePort. 웹과 포트가 다르므로 따로 읽는다.
	// loadbalancer 모드면 LBIP 로 22 번에 바로 붙으므로 이 값은 쓰지 않는다.
	SSHNodePort int
}

// sshSvcName은 SSH 전용 보조 Service 이름이다.
// 게이트웨이를 켜면 세션 Service 가 ClusterIP 라 사내망에서 컨테이너로 바로 SSH 할 수단이 사라진다.
// Service 는 포트별로 타입을 나눌 수 없으므로, 22 번만 담은 NodePort Service 를 따로 하나 더 만든다.
// 웹 채널은 그대로 ClusterIP 에 남아 게이트웨이만 거친다.
func sshSvcName(name string) string { return name + "-ssh" }

func svcType(mode string) corev1.ServiceType {
	if mode == ExposeLoadBalancer {
		return corev1.ServiceTypeLoadBalancer
	}
	return corev1.ServiceTypeNodePort // 기본 nodeport
}

// EnsureSessionService는 세션 Service 를 멱등 생성한다.
// Internal(게이트웨이 라우팅)이면 외부 노출 대신 ClusterIP Service 를 만들어 인클러스터 DNS 로 도달한다.
func (c *Client) EnsureSessionService(ctx context.Context, s SvcSpec) error {
	if !c.Available() || len(s.Ports) == 0 {
		return nil
	}
	svcTy := svcType(s.Mode)
	if s.Internal {
		svcTy = corev1.ServiceTypeClusterIP // 게이트웨이가 인클러스터 DNS 로 도달
	}
	ports := make([]corev1.ServicePort, 0, len(s.Ports))
	for _, p := range s.Ports {
		ports = append(ports, corev1.ServicePort{
			Name: p.Name, Port: int32(p.Port), TargetPort: intstr.FromInt(p.Port),
		})
	}
	svc := &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name: s.Name, Namespace: s.Namespace,
			Labels: map[string]string{"managed-by": "giosk-system", "giosk.io/session": s.Name},
		},
		Spec: corev1.ServiceSpec{
			Type:     svcTy,
			Selector: map[string]string{"giosk.io/session": s.Name},
			Ports:    ports,
		},
	}
	_, err := c.cs.CoreV1().Services(s.Namespace).Create(ctx, svc, v1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	if s.Internal {
		return c.ensureSSHService(ctx, s)
	}
	return nil
}

// ensureSSHService는 게이트웨이 모드에서 22 번만 NodePort 로 여는 보조 Service 를 만든다.
// LoadBalancer 를 쓰지 않는 이유는 세션마다 IP 를 하나씩 먹어 MetalLB 풀이 금방 마르기 때문이다.
func (c *Client) ensureSSHService(ctx context.Context, s SvcSpec) error {
	var ssh *SvcPort
	for i := range s.Ports {
		if s.Ports[i].Name == "ssh" {
			ssh = &s.Ports[i]
			break
		}
	}
	if ssh == nil {
		return nil
	}
	svc := &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name: sshSvcName(s.Name), Namespace: s.Namespace,
			Labels: map[string]string{"managed-by": "giosk-system", "giosk.io/session": s.Name},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: map[string]string{"giosk.io/session": s.Name},
			Ports: []corev1.ServicePort{{
				Name: "ssh", Port: int32(ssh.Port), TargetPort: intstr.FromInt(ssh.Port),
			}},
		},
	}
	_, err := c.cs.CoreV1().Services(s.Namespace).Create(ctx, svc, v1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// DeleteSessionService는 세션 Service 를 삭제한다(없어도 성공).
func (c *Client) DeleteSessionService(ctx context.Context, ns, name string) error {
	if !c.Available() {
		return nil
	}
	if err := c.cs.CoreV1().Services(ns).Delete(ctx, sshSvcName(name), v1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	err := c.cs.CoreV1().Services(ns).Delete(ctx, name, v1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// SessionServiceAccess는 Service 의 외부 접속 좌표(LB IP / NodePort)를 읽는다.
func (c *Client) SessionServiceAccess(ctx context.Context, ns, name, mode string) (SvcAccess, error) {
	out := SvcAccess{Mode: mode}
	if !c.Available() {
		return out, nil
	}
	svc, err := c.cs.CoreV1().Services(ns).Get(ctx, name, v1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if len(svc.Spec.Ports) > 0 {
		out.NodePort = int(svc.Spec.Ports[0].NodePort)
	}
	for _, p := range svc.Spec.Ports {
		if p.Name == "ssh" {
			out.SSHNodePort = int(p.NodePort)
			break
		}
	}
	for _, ing := range svc.Status.LoadBalancer.Ingress {
		if ing.IP != "" {
			out.LBIP = ing.IP
			break
		}
	}
	if out.SSHNodePort == 0 {
		// 게이트웨이 모드면 22 번은 보조 Service 에 있다.
		if aux, err := c.cs.CoreV1().Services(ns).Get(ctx, sshSvcName(name), v1.GetOptions{}); err == nil {
			for _, p := range aux.Spec.Ports {
				if p.Name == "ssh" {
					out.SSHNodePort = int(p.NodePort)
					break
				}
			}
		}
	}
	return out, nil
}

// NodeIP는 노드 이름의 InternalIP 를 반환한다(없으면 이름 그대로 폴백).
// 노드 이름은 클러스터 DNS 에 없어(lookup 실패) SSH/웹터미널이 노드에 붙을 때 IP 가 필요하다.
func (c *Client) NodeIP(ctx context.Context, node string) string {
	if !c.Available() || node == "" {
		return node
	}
	n, err := c.cs.CoreV1().Nodes().Get(ctx, node, v1.GetOptions{})
	if err != nil {
		return node
	}
	for _, a := range n.Status.Addresses {
		if a.Type == corev1.NodeInternalIP {
			return a.Address
		}
	}
	return node
}

// FirstNodeIP는 NodePort 접속용 워커 노드 IP 하나를 반환한다.
func (c *Client) FirstNodeIP(ctx context.Context) string {
	if !c.Available() {
		return ""
	}
	nodes, err := c.cs.CoreV1().Nodes().List(ctx, v1.ListOptions{})
	if err != nil {
		return ""
	}
	for i := range nodes.Items {
		if isControlPlane(&nodes.Items[i]) {
			continue
		}
		for _, a := range nodes.Items[i].Status.Addresses {
			if a.Type == corev1.NodeInternalIP {
				return a.Address
			}
		}
	}
	return ""
}
