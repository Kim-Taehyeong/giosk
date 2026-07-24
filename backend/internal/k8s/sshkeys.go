package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// userKeysDataKey는 Secret 안의 데이터 키이자 마운트된 파일 이름이다.
// sshd_config 의 AuthorizedKeysFile 두 번째 경로(/etc/giosk/ssh/authorized_keys)와 반드시 일치해야 한다.
const userKeysDataKey = "authorized_keys"

// UserKeysSecretName은 사용자별 authorized_keys Secret 이름이다(세션 네임스페이스 안에서 유일).
func UserKeysSecretName(userID int64) string { return fmt.Sprintf("giosk-sshkeys-%d", userID) }

// UpsertUserKeys는 사용자 등록 공개키를 담은 Secret 을 만들거나 갱신한다.
// sshd 사이드카가 이 Secret 을 read-only 로 마운트하고, sshd 는 접속마다 authorized_keys 를
// 다시 읽으므로 여기서 갱신하면 "실행 중인 세션"에도 즉시 반영된다(재시작 불필요).
// keys 가 비면 빈 파일을 쓴다 — 키를 지운 경우 즉시 로그인이 막혀야 하므로 Secret 을 남기되 비운다.
func (c *Client) UpsertUserKeys(ctx context.Context, ns string, userID int64, keys string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	if err := c.EnsureNamespace(ctx, ns); err != nil {
		return err
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      UserKeysSecretName(userID),
			Namespace: ns,
			Labels:    map[string]string{"managed-by": "giosk-system"},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{userKeysDataKey: []byte(keys)},
	}
	_, err := c.cs.CoreV1().Secrets(ns).Create(ctx, sec, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		_, err = c.cs.CoreV1().Secrets(ns).Update(ctx, sec, metav1.UpdateOptions{})
	}
	return err
}
