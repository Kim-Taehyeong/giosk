package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// sessionEgressPolicyName은 세션 네임스페이스마다 두는 이그레스 제한 정책 이름이다.
const sessionEgressPolicyName = "giosk-session-egress"

// SessionEgressSpec은 세션 파드가 나갈 수 있는 곳을 제한하는 정책 입력이다.
//
// 세션 파드는 사용자가 임의 코드를 돌리는 곳이라, 기본 상태에서는 사내망 전체(스토리지 NFS,
// 노드 kubelet, API 서버, 다른 팀 클러스터)에 그대로 닿는다. 마운트 경로에서 스토리지 주소를
// 가려 봐야 포트 스캔 몇 초면 다시 찾아내므로, 실질적인 차단은 여기서 한다.
//
// 볼륨 마운트는 kubelet(호스트 네트워크 네임스페이스)이 수행하므로 파드 이그레스를 막아도
// NFS 볼륨과 데이터셋은 영향받지 않는다. 막히는 것은 파드가 스스로 여는 연결뿐이다.
type SessionEgressSpec struct {
	Namespace string
	// DenyCIDRs는 나가지 못하게 할 대역이다(사내망·스토리지·노드망). 비면 정책을 만들지 않는다.
	DenyCIDRs []string
	// AllowCIDRs는 DenyCIDRs 안에서도 예외로 열어 줄 대역이다(사내 패키지 미러 등).
	AllowCIDRs []string
	// DNSServiceIP는 클러스터 DNS Service IP(예: 10.96.0.10)다. CNI 가 DNAT 이전 주소로 정책을
	// 평가하는 경우를 대비해 명시로 열어 준다. 비어 있으면 kube-dns 파드 셀렉터 규칙만 쓴다.
	DNSServiceIP string
}

// EnsureSessionEgressPolicy는 세션 파드 전용 이그레스 제한 정책을 멱등 적용한다(있으면 갱신).
// 대상은 giosk.io/session 라벨이 있는 파드뿐이라, 같은 네임스페이스의 데이터셋·빌드 Job 은 영향받지 않는다.
// DenyCIDRs 가 비면 정책을 지운다(기능을 끈 상태로 되돌리기).
func (c *Client) EnsureSessionEgressPolicy(ctx context.Context, s SessionEgressSpec) error {
	if !c.Available() {
		return ErrNoCluster
	}
	if len(s.DenyCIDRs) == 0 {
		return c.deleteSessionEgressPolicy(ctx, s.Namespace)
	}
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sessionEgressPolicyName,
			Namespace: s.Namespace,
			Labels:    map[string]string{"managed-by": "giosk-system"},
		},
		Spec: networkingv1.NetworkPolicySpec{
			// 세션 파드만 대상으로 한다. 같은 ns 의 데이터셋 적재 Job 등은 사내 자원에 닿아야 한다.
			PodSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key: "giosk.io/session", Operator: metav1.LabelSelectorOpExists,
				}},
			},
			// Egress 만 건다. Ingress 를 함께 막으면 NodePort·게이트웨이로 들어오는 사용자 접속이 끊긴다.
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress:      sessionEgressRules(s),
		},
	}
	_, err := c.cs.NetworkingV1().NetworkPolicies(s.Namespace).Create(ctx, np, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		// 설정(차단 대역)이 바뀌었을 수 있으므로 갱신한다.
		cur, getErr := c.cs.NetworkingV1().NetworkPolicies(s.Namespace).Get(ctx, sessionEgressPolicyName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		cur.Spec = np.Spec
		_, err = c.cs.NetworkingV1().NetworkPolicies(s.Namespace).Update(ctx, cur, metav1.UpdateOptions{})
	}
	return err
}

// sessionEgressRules는 허용 규칙 목록을 만든다. NetworkPolicy 는 Egress 규칙이 하나라도 있으면
// 그 규칙에 맞지 않는 모든 이그레스를 막으므로, "열어 줄 것"만 나열하면 나머지는 자동으로 차단된다.
func sessionEgressRules(s SessionEgressSpec) []networkingv1.NetworkPolicyEgressRule {
	// 1) 인터넷은 열되 사내 대역만 도려낸다. pip·conda·HuggingFace 내려받기가 막히면 안 된다.
	rules := []networkingv1.NetworkPolicyEgressRule{{
		To: []networkingv1.NetworkPolicyPeer{{
			IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0", Except: s.DenyCIDRs},
		}},
	}}
	// 2) 사내 대역 중 명시로 허용한 곳(사내 미러 등).
	for _, cidr := range s.AllowCIDRs {
		if cidr == "" {
			continue
		}
		rules = append(rules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{{IPBlock: &networkingv1.IPBlock{CIDR: cidr}}},
		})
	}
	// 3) DNS. 이름 해석이 막히면 인터넷을 열어 둔 의미가 없다.
	//    kube-dns 파드는 파드 대역(보통 차단 대상)에 있으므로 셀렉터로 따로 열어 준다.
	dnsPorts := []networkingv1.NetworkPolicyPort{
		{Protocol: protoPtr(corev1.ProtocolUDP), Port: portPtr(53)},
		{Protocol: protoPtr(corev1.ProtocolTCP), Port: portPtr(53)},
	}
	dnsPeers := []networkingv1.NetworkPolicyPeer{{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": "kube-system"},
		},
		PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"k8s-app": "kube-dns"}},
	}}
	if s.DNSServiceIP != "" {
		dnsPeers = append(dnsPeers, networkingv1.NetworkPolicyPeer{
			IPBlock: &networkingv1.IPBlock{CIDR: s.DNSServiceIP + "/32"},
		})
	}
	return append(rules, networkingv1.NetworkPolicyEgressRule{To: dnsPeers, Ports: dnsPorts})
}

// deleteSessionEgressPolicy는 정책을 제거한다(없어도 성공). 기능을 껐을 때 잔재를 남기지 않는다.
func (c *Client) deleteSessionEgressPolicy(ctx context.Context, ns string) error {
	err := c.cs.NetworkingV1().NetworkPolicies(ns).Delete(ctx, sessionEgressPolicyName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func protoPtr(p corev1.Protocol) *corev1.Protocol { return &p }

func portPtr(n int32) *intstr.IntOrString { v := intstr.FromInt32(n); return &v }
