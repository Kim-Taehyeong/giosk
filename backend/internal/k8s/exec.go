package k8s

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// TermSize는 웹터미널 창 크기(열/행).
type TermSize struct{ Cols, Rows uint16 }

// ExecIO는 웹터미널 exec 의 입출력 배선. Resize 는 창 크기 변경 이벤트(닫히면 종료).
type ExecIO struct {
	Stdin  io.Reader
	Stdout io.Writer
	Resize <-chan TermSize
}

// ExecTerminal은 세션 파드 컨테이너에 TTY exec 로 붙어 stdin/stdout 을 스트리밍한다(kubectl exec -it 동등).
// 웹터미널(브라우저 xterm ↔ API websocket ↔ 여기)의 컨테이너 세션 경로.
func (c *Client) ExecTerminal(ctx context.Context, ns, pod, container string, cmd []string, tio ExecIO) error {
	if !c.Available() {
		return ErrNoCluster
	}
	req := c.cs.CoreV1().RESTClient().Post().
		Resource("pods").Name(pod).Namespace(ns).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   cmd,
			Stdin:     tio.Stdin != nil,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(c.cfg, "POST", req.URL())
	if err != nil {
		return err
	}
	return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             tio.Stdin,
		Stdout:            tio.Stdout,
		Stderr:            tio.Stdout, // TTY 는 stdout/stderr 를 한 스트림으로 합친다
		Tty:               true,
		TerminalSizeQueue: resizeQueue{tio.Resize},
	})
}

// resizeQueue는 TermSize 채널을 remotecommand.TerminalSizeQueue 로 어댑트한다(채널 닫힘=nil→종료).
type resizeQueue struct{ ch <-chan TermSize }

func (r resizeQueue) Next() *remotecommand.TerminalSize {
	if r.ch == nil {
		return nil
	}
	s, ok := <-r.ch
	if !ok {
		return nil
	}
	return &remotecommand.TerminalSize{Width: s.Cols, Height: s.Rows}
}
