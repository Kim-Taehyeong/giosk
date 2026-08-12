package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// 이그레스 규칙의 핵심은 "사내망은 막고 인터넷과 DNS 는 연다"이다. 셋 중 하나라도 어긋나면
// 세션이 못 쓰게 되거나(인터넷·DNS 차단) 보안 목적을 잃는다(사내망 개방).
func TestSessionEgressRules(t *testing.T) {
	deny := []string{"10.0.0.0/8", "192.168.0.0/16"}
	rules := sessionEgressRules(SessionEgressSpec{
		Namespace: "giosk-grp-1", DenyCIDRs: deny,
		AllowCIDRs: []string{"10.20.30.40/32"}, DNSServiceIP: "10.96.0.10",
	})

	// 1) 인터넷은 열되 차단 대역은 도려낸다.
	first := rules[0].To[0].IPBlock
	if first == nil || first.CIDR != "0.0.0.0/0" {
		t.Fatalf("첫 규칙이 인터넷 허용이 아니다: %+v", rules[0].To[0])
	}
	if len(first.Except) != len(deny) {
		t.Errorf("차단 대역이 except 로 안 들어갔다: %v", first.Except)
	}

	// 2) 사내망 예외(미러 등)가 그대로 열려 있다.
	foundAllow := false
	for _, r := range rules {
		for _, p := range r.To {
			if p.IPBlock != nil && p.IPBlock.CIDR == "10.20.30.40/32" {
				foundAllow = true
			}
		}
	}
	if !foundAllow {
		t.Error("AllowCIDRs 예외 규칙이 없다")
	}

	// 3) DNS 는 kube-dns 셀렉터와 Service IP 양쪽으로 열려 있고 53 포트로 제한된다.
	last := rules[len(rules)-1]
	if len(last.Ports) != 2 {
		t.Fatalf("DNS 규칙 포트가 UDP/TCP 53 두 개가 아니다: %+v", last.Ports)
	}
	for _, p := range last.Ports {
		if p.Port == nil || p.Port.IntValue() != 53 {
			t.Errorf("DNS 포트가 53 이 아니다: %+v", p)
		}
		if p.Protocol == nil || (*p.Protocol != corev1.ProtocolUDP && *p.Protocol != corev1.ProtocolTCP) {
			t.Errorf("DNS 프로토콜이 UDP/TCP 가 아니다: %+v", p.Protocol)
		}
	}
	hasSelector, hasServiceIP := false, false
	for _, p := range last.To {
		if p.PodSelector != nil && p.PodSelector.MatchLabels["k8s-app"] == "kube-dns" {
			hasSelector = true
		}
		if p.IPBlock != nil && p.IPBlock.CIDR == "10.96.0.10/32" {
			hasServiceIP = true
		}
	}
	if !hasSelector || !hasServiceIP {
		t.Errorf("DNS 허용이 부족하다(셀렉터=%v ServiceIP=%v)", hasSelector, hasServiceIP)
	}
}

// DNS Service IP 를 안 줘도 kube-dns 셀렉터만으로 이름 해석이 열려 있어야 한다.
func TestSessionEgressRulesWithoutDNSServiceIP(t *testing.T) {
	rules := sessionEgressRules(SessionEgressSpec{DenyCIDRs: []string{"10.0.0.0/8"}})
	last := rules[len(rules)-1]
	if len(last.To) != 1 || last.To[0].PodSelector == nil {
		t.Errorf("DNS 셀렉터 규칙이 없다: %+v", last.To)
	}
}
