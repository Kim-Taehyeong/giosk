package k8s

import (
	"context"
	"io"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// kanikoImage는 빌드 실행기(Kaniko executor). 컨텍스트=ConfigMap 의 Dockerfile.
const kanikoImage = "gcr.io/kaniko-project/executor:latest"

// controlPlaneSelector는 빌드/스캔/서명 Job 을 control-plane(마스터) 노드에 스케줄하는 nodeSelector.
// GPU 워커를 점유하지 않게 마스터에서 돌린다. (배포에 따라 라벨이 control-plane 이 아니라 master 일 수
// 있으나 최신 k8s 는 control-plane 을 부여한다.)
func controlPlaneSelector() map[string]string {
	return map[string]string{"node-role.kubernetes.io/control-plane": ""}
}

// controlPlaneTolerations는 control-plane 의 표준 taint(control-plane/master)를 견디게 한다.
func controlPlaneTolerations() []corev1.Toleration {
	return []corev1.Toleration{
		{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
		{Key: "node-role.kubernetes.io/master", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
	}
}

// BuildSpec은 이미지 빌드 Job 입력.
type BuildSpec struct {
	Namespace  string // 빌드 Job 네임스페이스
	JobName    string // build-<imageID>
	ImageRef   string // 푸시 대상 = <registry>/<name>:<tag>
	Dockerfile string // 생성된 Dockerfile 내용
	Registry   string // insecure 처리할 레지스트리 호스트(예: giosk-registry:5000)
}

// 빌드 Job 상태.
const (
	BuildRunning   = "running"
	BuildSucceeded = "succeeded"
	BuildFailed    = "failed"
)

// RunBuildJob은 Dockerfile 을 ConfigMap 컨텍스트로 올리고 Kaniko Job 을 생성한다(멱등 재생성).
func (c *Client) RunBuildJob(ctx context.Context, s BuildSpec) error {
	if !c.Available() {
		return ErrNoCluster
	}
	if err := c.EnsureNamespace(ctx, s.Namespace); err != nil {
		return err
	}
	cmName := s.JobName + "-ctx"
	// 이전 동일 이름 잔재 정리(rebuild 재사용).
	_ = c.DeleteBuildJob(ctx, s.Namespace, s.JobName)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: s.Namespace, Labels: map[string]string{"managed-by": "giosk-system"}},
		Data:       map[string]string{"Dockerfile": s.Dockerfile},
	}
	if _, err := c.cs.CoreV1().ConfigMaps(s.Namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	args := []string{
		"--dockerfile=/workspace/Dockerfile",
		"--context=dir:///workspace",
		"--destination=" + s.ImageRef,
		"--cache=false",
	}
	if s.Registry != "" { // 인클러스터 insecure 레지스트리만 http 허용(베이스 풀은 https 유지)
		args = append(args,
			"--insecure-registry="+s.Registry,
			"--skip-tls-verify-registry="+s.Registry)
	}
	backoff := int32(0)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: s.JobName, Namespace: s.Namespace, Labels: map[string]string{"managed-by": "giosk-system", "giosk.io/build": "true"}},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector:  controlPlaneSelector(), // 빌드는 마스터(control-plane) 노드에서 — GPU 워커 점유 회피
					Tolerations:   controlPlaneTolerations(),
					Containers: []corev1.Container{{
						Name:         "kaniko",
						Image:        kanikoImage,
						Args:         args,
						VolumeMounts: []corev1.VolumeMount{{Name: "ctx", MountPath: "/workspace"}},
					}},
					Volumes: []corev1.Volume{{
						Name: "ctx",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: cmName}},
						},
					}},
				},
			},
		},
	}
	_, err := c.cs.BatchV1().Jobs(s.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// RunScanJob은 trivy 로 이미지 취약점을 스캔하는 Job 을 생성한다(HIGH/CRITICAL 발견 시 exit 1 → Failed).
func (c *Client) RunScanJob(ctx context.Context, ns, name, imageRef, registry string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	if err := c.EnsureNamespace(ctx, ns); err != nil {
		return err
	}
	_ = c.DeleteBuildJob(ctx, ns, name)
	args := []string{"image", "--severity", "HIGH,CRITICAL", "--exit-code", "1", "--quiet", "--timeout", "10m", "--no-progress"}
	if registry != "" {
		args = append(args, "--insecure") // 인클러스터 insecure 레지스트리 풀 허용
	}
	args = append(args, imageRef)
	return c.runSimpleJob(ctx, ns, name, "aquasec/trivy:latest", args, nil, nil)
}

// RunDatasetFetch는 NFS 정규경로 <nfsBase>/dataset/<name> 를 만들고(mkdir) URL 을 그 안으로 적재한다.
// NFS 베이스를 직접 마운트(/nfs)해 서브디렉터리를 생성하므로, 물리노드도 같은 경로를 바로 마운트할 수 있다.
// url 이 비면 빈 디렉터리만 생성한다(빈 데이터셋). curlimages/curl 은 busybox sh 를 포함한다.
func (c *Client) RunDatasetFetch(ctx context.Context, ns, jobName, nfsServer, nfsBase, name, url string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	if err := c.EnsureNamespace(ctx, ns); err != nil {
		return err
	}
	_ = c.DeleteBuildJob(ctx, ns, jobName)
	dir := "/nfs/dataset/" + name
	// 백그라운드 curl + 2초마다 파일 크기를 "PROGRESS <bytes>" 로 로그에 출력 → API 가 로그를 파싱해 진행률(%) 산출.
	// (Prometheus 스크랩 주기에 의존하지 않는 실측 진행률). 다운로드 후 아카이브면 제자리 해제하되
	// 원본 아카이브는 보존한다 — slow 마운트는 이 폴더를 그대로 쓰므로 해제본이 NFS 에 있어야 세션이 바로
	// 읽고, fast 캐싱은 이 아카이브(zip) 하나만 노드로 복사해 로컬에서 해제한다(대용량 다중파일 복사 회피).
	script := "set -e; mkdir -p '" + dir + "'"
	if url != "" {
		script += `
apk add --no-cache curl unzip tar >/dev/null 2>&1 || true
fn=$(basename "` + url + `" | sed 's/[?].*//'); [ -z "$fn" ] && fn=download.bin
( curl -fSL --retry 3 -o "` + dir + `/$fn" "` + url + `" ) &
pid=$!
while kill -0 "$pid" 2>/dev/null; do
  sz=$(stat -c %s "` + dir + `/$fn" 2>/dev/null || echo 0)
  echo "PROGRESS $sz"
  sleep 2
done
wait "$pid"
echo "PROGRESS DONE"
case "$fn" in
  *.zip) echo "EXTRACT zip"; unzip -o -q "` + dir + `/$fn" -d "` + dir + `" || echo "extract failed(아카이브만 보관)" ;;
  *.tar.gz|*.tgz) echo "EXTRACT tar.gz"; tar -xzf "` + dir + `/$fn" -C "` + dir + `" || echo "extract failed(아카이브만 보관)" ;;
  *.tar) echo "EXTRACT tar"; tar -xf "` + dir + `/$fn" -C "` + dir + `" || echo "extract failed(아카이브만 보관)" ;;
  *) echo "no-extract(단일파일 데이터셋)" ;;
esac
echo "EXTRACT DONE"`
	}
	vol := []corev1.Volume{{Name: "nfs", VolumeSource: corev1.VolumeSource{NFS: &corev1.NFSVolumeSource{Server: nfsServer, Path: nfsBase}}}}
	mounts := []corev1.VolumeMount{{Name: "nfs", MountPath: "/nfs"}}
	// alpine: apk 로 curl/unzip/tar 확보(root 실행 → apk 가능; curlimages 는 non-root 라 apk 불가).
	return c.runSimpleJobCmd(ctx, ns, jobName, "alpine:3.20", []string{"sh", "-c", script}, nil, nil, vol, mounts)
}

// RunDatasetCache는 특정 노드에 핀되어 데이터셋 NFS(<nfsServer>:<nfsDatasetPath>, RO)를
// 그 노드의 로컬 디스크 hostPath(<hostBase>/<name>)로 복사하는 Job 을 만든다(노드 로컬 캐시).
// 세션은 캐시된 노드에서 이 hostPath 를 직접 마운트해 NFS 보다 빠르게 접근한다.
func (c *Client) RunDatasetCache(ctx context.Context, ns, jobName, node, nfsServer, nfsDatasetPath, hostBase, name string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	if err := c.EnsureNamespace(ctx, ns); err != nil {
		return err
	}
	_ = c.DeleteBuildJob(ctx, ns, jobName)
	dst := "/dst/" + name
	// fast 캐싱: NFS(/src)에는 아카이브(zip 등) + 해제본이 함께 있다. 해제본을 통째로 복사하면
	// (cp -a) 수많은 작은 파일이 NFS 를 오가 느리다 → 아카이브 1개만 노드로 복사(단일 대용량=빠름)한 뒤
	// 노드 로컬에서 해제한다. 아카이브가 없으면(단일파일 데이터셋) 폴더 통째 복사로 폴백.
	// 복사 진행률을 "PROGRESS <cur> <total>" 로 출력 → API 가 파싱해 %.
	script := "set -e; apk add --no-cache unzip tar >/dev/null 2>&1 || true; mkdir -p '" + dst + `'
arc=$(ls /src/*.zip /src/*.tar.gz /src/*.tgz /src/*.tar 2>/dev/null | head -1)
if [ -n "$arc" ]; then
  bn=$(basename "$arc"); total=$(stat -c %s "$arc" 2>/dev/null || echo 0)
  ( cp "$arc" '` + dst + `/'"$bn" ) &
  pid=$!
  while kill -0 "$pid" 2>/dev/null; do
    cur=$(stat -c %s '` + dst + `/'"$bn" 2>/dev/null || echo 0)
    echo "PROGRESS $cur $total"; sleep 2
  done
  wait "$pid"
  echo "PROGRESS $total $total"
  echo "EXTRACT"
  case "$bn" in
    *.zip) unzip -o -q '` + dst + `/'"$bn" -d '` + dst + `' ;;
    *.tar.gz|*.tgz) tar -xzf '` + dst + `/'"$bn" -C '` + dst + `' ;;
    *.tar) tar -xf '` + dst + `/'"$bn" -C '` + dst + `' ;;
  esac
  rm -f '` + dst + `/'"$bn"
else
  cp -a /src/. '` + dst + `/'
fi
echo cached`
	hostType := corev1.HostPathDirectoryOrCreate
	vols := []corev1.Volume{
		{Name: "src", VolumeSource: corev1.VolumeSource{NFS: &corev1.NFSVolumeSource{Server: nfsServer, Path: nfsDatasetPath, ReadOnly: true}}},
		{Name: "dst", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: hostBase, Type: &hostType}}},
	}
	mounts := []corev1.VolumeMount{
		{Name: "src", MountPath: "/src", ReadOnly: true},
		{Name: "dst", MountPath: "/dst"},
	}
	return c.runSimpleJobNode(ctx, ns, jobName, node, "alpine:3.20", []string{"sh", "-c", script}, vols, mounts)
}

// RunDatasetUncache는 특정 노드의 로컬 캐시 디렉터리(<hostBase>/<name>)를 삭제하는 Job 을 만든다.
func (c *Client) RunDatasetUncache(ctx context.Context, ns, jobName, node, hostBase, name string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	if err := c.EnsureNamespace(ctx, ns); err != nil {
		return err
	}
	_ = c.DeleteBuildJob(ctx, ns, jobName)
	hostType := corev1.HostPathDirectoryOrCreate
	vols := []corev1.Volume{{Name: "dst", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: hostBase, Type: &hostType}}}}
	mounts := []corev1.VolumeMount{{Name: "dst", MountPath: "/dst"}}
	return c.runSimpleJobNode(ctx, ns, jobName, node, "busybox:1.36", []string{"sh", "-c", "rm -rf '/dst/" + name + "'; echo removed"}, vols, mounts)
}

// runSimpleJobNode는 특정 노드(NodeName)에 핀된 일회성 Job 을 만든다(hostPath 작업용).
func (c *Client) runSimpleJobNode(ctx context.Context, ns, name, node, image string, command []string, vols []corev1.Volume, mounts []corev1.VolumeMount) error {
	backoff := int32(0)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{"managed-by": "giosk-system"}},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeName:      node, // 해당 노드 로컬 디스크에 직접 작업
					RestartPolicy: corev1.RestartPolicyNever,
					Containers:    []corev1.Container{{Name: "job", Image: image, Command: command, VolumeMounts: mounts}},
					Volumes:       vols,
				},
			},
		},
	}
	_, err := c.cs.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// runSimpleJobCmd는 runSimpleJobVol 과 같되 entrypoint(Command)까지 덮어쓴다(쉘 스크립트 실행용).
func (c *Client) runSimpleJobCmd(ctx context.Context, ns, name, image string, command []string, args []string, env []corev1.EnvVar, vols []corev1.Volume, mounts []corev1.VolumeMount) error {
	backoff := int32(0)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{"managed-by": "giosk-system"}},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers:    []corev1.Container{{Name: "job", Image: image, Command: command, Args: args, Env: env, VolumeMounts: mounts}},
					Volumes:       vols,
				},
			},
		},
	}
	_, err := c.cs.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// RunSignJob은 cosign 으로 이미지를 서명하는 Job 을 생성한다(keySecret: cosign.key + password).
func (c *Client) RunSignJob(ctx context.Context, ns, name, imageRef, keySecret, registry string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	if err := c.EnsureNamespace(ctx, ns); err != nil {
		return err
	}
	_ = c.DeleteBuildJob(ctx, ns, name)
	// --tlog-upload=false: 공개 Rekor(rekor.sigstore.dev) 미접속(인클러스터/에어갭). 서명은 레지스트리에 저장.
	args := []string{"sign", "--key", "/keys/cosign.key", "--yes", "--tlog-upload=false"}
	if registry != "" { // 사설 평문 HTTP 레지스트리: TLS 검증 생략 + HTTP 허용.
		args = append(args, "--allow-insecure-registry", "--allow-http-registry")
	}
	args = append(args, imageRef)
	vol := []corev1.Volume{{Name: "key", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: keySecret}}}}
	mounts := []corev1.VolumeMount{{Name: "key", MountPath: "/keys", ReadOnly: true}}
	optional := true
	env := []corev1.EnvVar{{Name: "COSIGN_PASSWORD", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: keySecret}, Key: "cosign.password", Optional: &optional}}}}
	return c.runSimpleJobVol(ctx, ns, name, "gcr.io/projectsigstore/cosign:v2.2.4", args, env, vol, mounts)
}

// EnsureCosignKey는 서명키 Secret 이 없으면 cosign generate-key-pair 로 in-cluster 생성한다.
// 반환 ready=true 이면 키가 준비된 것(서명 가능), false 면 생성 Job 이 진행 중(다음 주기 재시도).
// 생성된 Secret 은 cosign 표준 키(cosign.key/cosign.password/cosign.pub)를 가지며 암호는 빈 문자열.
func (c *Client) EnsureCosignKey(ctx context.Context, ns, secret string) (bool, error) {
	if !c.Available() {
		return false, ErrNoCluster
	}
	if err := c.EnsureNamespace(ctx, ns); err != nil {
		return false, err
	}
	if _, err := c.cs.CoreV1().Secrets(ns).Get(ctx, secret, metav1.GetOptions{}); err == nil {
		return true, nil // 이미 준비됨
	} else if !apierrors.IsNotFound(err) {
		return false, err
	}
	if err := c.ensureKeygenRBAC(ctx, ns); err != nil {
		return false, err
	}
	// k8s:// 출력은 default SA 가 해당 ns 에서 Secret 을 생성할 권한을 필요로 한다(위 RBAC).
	args := []string{"generate-key-pair", "k8s://" + ns + "/" + secret}
	env := []corev1.EnvVar{{Name: "COSIGN_PASSWORD", Value: ""}}
	if err := c.runKeygenJob(ctx, ns, "cosign-keygen-"+secret, args, env); err != nil {
		return false, err
	}
	return false, nil // 생성 진행 중
}

// ensureKeygenRBAC는 default SA 가 cosign keygen 으로 Secret 을 쓸 수 있도록 Role/RoleBinding 을 보장한다.
func (c *Client) ensureKeygenRBAC(ctx context.Context, ns string) error {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "giosk-cosign-keygen", Namespace: ns, Labels: map[string]string{"managed-by": "giosk-system"}},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""}, Resources: []string{"secrets"},
			Verbs: []string{"get", "create", "update", "patch"},
		}},
	}
	if _, err := c.cs.RbacV1().Roles(ns).Create(ctx, role, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "giosk-cosign-keygen", Namespace: ns, Labels: map[string]string{"managed-by": "giosk-system"}},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "default", Namespace: ns}},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "giosk-cosign-keygen"},
	}
	if _, err := c.cs.RbacV1().RoleBindings(ns).Create(ctx, rb, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// runKeygenJob은 default SA 로 실행되는 일회성 cosign keygen Job 을 만든다(이미 있으면 무시).
func (c *Client) runKeygenJob(ctx context.Context, ns, name string, args []string, env []corev1.EnvVar) error {
	backoff := int32(1)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{"managed-by": "giosk-system"}},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: "default",
					Containers:         []corev1.Container{{Name: "keygen", Image: "gcr.io/projectsigstore/cosign:v2.2.4", Args: args, Env: env}},
				},
			},
		},
	}
	_, err := c.cs.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// runSimpleJob은 컨테이너 1개짜리 일회성 Job 을 생성한다(backoff 0, RestartNever).
func (c *Client) runSimpleJob(ctx context.Context, ns, name, image string, args []string, env []corev1.EnvVar, _ []corev1.Volume) error {
	return c.runSimpleJobVol(ctx, ns, name, image, args, env, nil, nil)
}

func (c *Client) runSimpleJobVol(ctx context.Context, ns, name, image string, args []string, env []corev1.EnvVar, vols []corev1.Volume, mounts []corev1.VolumeMount) error {
	backoff := int32(0)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{"managed-by": "giosk-system"}},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector:  controlPlaneSelector(), // 스캔/서명도 빌드 파이프라인 → 마스터에서
					Tolerations:   controlPlaneTolerations(),
					Containers:    []corev1.Container{{Name: "job", Image: image, Args: args, Env: env, VolumeMounts: mounts}},
					Volumes:       vols,
				},
			},
		},
	}
	_, err := c.cs.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// BuildJobStatus는 빌드 Job 의 상태(running/succeeded/failed)를 반환한다(없으면 failed).
func (c *Client) BuildJobStatus(ctx context.Context, ns, jobName string) (string, error) {
	if !c.Available() {
		return "", ErrNoCluster
	}
	job, err := c.cs.BatchV1().Jobs(ns).Get(ctx, jobName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return BuildFailed, nil
	}
	if err != nil {
		return "", err
	}
	if job.Status.Succeeded > 0 {
		return BuildSucceeded, nil
	}
	if job.Status.Failed > 0 {
		return BuildFailed, nil
	}
	return BuildRunning, nil
}

// DeleteBuildJob은 빌드 Job 과 컨텍스트 ConfigMap 을 삭제한다(없어도 성공).
func (c *Client) DeleteBuildJob(ctx context.Context, ns, jobName string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	policy := metav1.DeletePropagationBackground
	_ = c.cs.BatchV1().Jobs(ns).Delete(ctx, jobName, metav1.DeleteOptions{PropagationPolicy: &policy})
	_ = c.cs.CoreV1().ConfigMaps(ns).Delete(ctx, jobName+"-ctx", metav1.DeleteOptions{})
	return nil
}

// EnsurePrefetchPod는 노드에 이미지를 미리 풀하기 위한 경량 Pod 를 생성한다(멱등 재생성).
// 컨테이너 명령은 즉시 종료(이미지 풀이 목적). kubelet 이 노드 containerd 로 이미지를 당겨 캐시한다.
func (c *Client) EnsurePrefetchPod(ctx context.Context, ns, name, node, imageRef string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	if err := c.EnsureNamespace(ctx, ns); err != nil {
		return err
	}
	_ = c.cs.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{}) // 잔재 정리(재시도)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{"managed-by": "giosk-system", "giosk.io/prefetch": "true"}},
		Spec: corev1.PodSpec{
			NodeName:      node,
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:    "prefetch",
				Image:   imageRef,
				Command: []string{"/bin/sh", "-c", "exit 0"},
			}},
		},
	}
	_, err := c.cs.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// PrefetchStatus는 프리페치 Pod 상태를 캐시 상태(cached/failed/pulling)로 매핑한다.
// 이미지 풀 자체가 목적이므로, 풀 실패(ImagePull)만 failed, 그 외(완료/실행/명령실패)는 cached 로 본다.
func (c *Client) PrefetchStatus(ctx context.Context, ns, name string) string {
	if !c.Available() {
		return "pulling"
	}
	p, err := c.cs.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) || err != nil {
		return "pulling"
	}
	for _, cs := range p.Status.ContainerStatuses {
		if w := cs.State.Waiting; w != nil && (w.Reason == "ImagePullBackOff" || w.Reason == "ErrImagePull") {
			return "failed"
		}
		if cs.State.Running != nil || cs.State.Terminated != nil { // 풀 완료 후 실행/종료 → 캐시됨
			return "cached"
		}
	}
	if p.Status.Phase == corev1.PodSucceeded {
		return "cached"
	}
	return "pulling"
}

// DeletePrefetchPod은 프리페치 Pod 를 삭제한다(없어도 성공). 노드의 이미지 레이어는 containerd GC 까지 잔존.
func (c *Client) DeletePrefetchPod(ctx context.Context, ns, name string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	err := c.cs.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// BuildLogs는 빌드 Job 파드의 로그 tail 을 가져온다(실패 진단용).
func (c *Client) BuildLogs(ctx context.Context, ns, jobName string, tail int64) string {
	if !c.Available() {
		return ""
	}
	pods, err := c.cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + jobName})
	if err != nil || len(pods.Items) == 0 {
		return ""
	}
	req := c.cs.CoreV1().Pods(ns).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{TailLines: &tail})
	stream, err := req.Stream(ctx)
	if err != nil {
		return ""
	}
	defer stream.Close()
	b, _ := io.ReadAll(stream)
	return string(b)
}
