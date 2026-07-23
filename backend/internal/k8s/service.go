package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// 세션 노출 모드.
const (
	ExposeLoadBalancer = "loadbalancer"
	ExposeNodePort     = "nodeport"
	ExposePortForward  = "portforward"
)

// SvcSpec은 세션 웹 노출 Service 입력.
type SvcSpec struct {
	Namespace string
	Name      string    // = instance_id (pod 와 동일; selector giosk.io/session)
	Ports     []SvcPort // 노출 포트(웹 채널 + sshd 22). 첫 항목이 primary(외부 노출 좌표).
	Mode      string    // loadbalancer | nodeport | portforward
	Internal  bool      // 게이트웨이 라우팅용: portforward 모드여도 ClusterIP Service 를 항상 생성.
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
	NodePort int    // nodeport 할당 포트
}

func svcType(mode string) corev1.ServiceType {
	switch mode {
	case ExposeLoadBalancer:
		return corev1.ServiceTypeLoadBalancer
	case ExposeNodePort:
		return corev1.ServiceTypeNodePort
	default:
		return corev1.ServiceTypeClusterIP
	}
}

// EnsureSessionService는 세션 Service 를 멱등 생성한다. portforward 모드면 생략하되,
// Internal(게이트웨이 라우팅)이면 ClusterIP Service 를 항상 만들어 모든 채널 포트(+sshd 22)를 노출한다.
func (c *Client) EnsureSessionService(ctx context.Context, s SvcSpec) error {
	if !c.Available() || len(s.Ports) == 0 {
		return nil
	}
	svcTy := svcType(s.Mode)
	if s.Mode == ExposePortForward {
		if !s.Internal {
			return nil // 외부 노출도 게이트웨이도 없음 → Service 불요(port-forward)
		}
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
	if !c.Available() || mode == ExposePortForward {
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
