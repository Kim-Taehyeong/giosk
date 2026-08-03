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
	return out, nil
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
