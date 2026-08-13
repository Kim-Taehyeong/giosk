package k8s

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GPU 확장 리소스 이름. 전용은 nvidia.com/gpu, 분할(HAMi)은 gpumem 과 gpucores 다.
const (
	resGPU      = "nvidia.com/gpu"
	resGPUMem   = "nvidia.com/gpumem"
	resGPUCores = "nvidia.com/gpucores"

	GpuModeShared    = "shared"    // HAMi 분할(VRAM·코어). 메모리 격리가 있다
	GpuModeExclusive = "exclusive" // GPU 카드 통째
	GpuModeTimeslice = "timeslice" // 타임슬라이싱 슬롯(GPU 1개를 N분할). 메모리 격리가 없다
	GpuModeCPU       = "cpu"
)

// ShareModeLabel은 노드의 공유 전략 라벨. 타임슬라이싱 노드는 GPU 를 N슬롯으로 광고하므로
// 전용 요청(nvidia.com/gpu:N)이 그 노드에 떨어지면 통 GPU 대신 슬롯을 받으므로 라벨로 배치를 가른다.
const ShareModeLabel = "giosk.io/share-mode"
const shareTimeslicing = "timeslicing"
const shareHami = "hami" // HAMi 분할 노드 라벨값(node.ShareHami 와 동일). 전용 배치에서 배제용.

// buildPod은 SessionSpec 을 corev1.Pod 로 변환한다(노드셀렉터와 GPU 리소스 매핑).
func buildPod(s SessionSpec, gpuTypeLabel string) *corev1.Pod {
	vols, mounts := podVolumes(s)
	// 세션은 사용자가 임의 코드를 돌리는 곳이라 쿠버네티스 API 토큰을 줄 이유가 없다.
	// 기본값이면 default SA 토큰이 마운트돼 API 서버에 도달할 수 있다. RBAC 이 막고 있더라도
	// 필요 없는 공격 표면이므로 아예 넣지 않는다(세션 컨테이너는 API 를 쓰지 않는다).
	noSAToken := false
	spec := corev1.PodSpec{
		NodeSelector:                 nodeSelector(s, gpuTypeLabel),
		Affinity:                     nodeAffinity(s.PreferNodes, s.RequireNode, shareModeReqs(s)),
		RestartPolicy:                corev1.RestartPolicyNever,
		AutomountServiceAccountToken: &noSAToken,
		Volumes:                      vols,
		Containers: []corev1.Container{{
			Name:  "session",
			Image: s.Image,
			// GPU 는 limit(스케줄 제약). CPU·메모리는 request 만 건다 = "최소 보장"이지 상한이 아니다.
			// limit 을 안 걸어야 노드가 한가할 때 그 이상 쓸 수 있다(큰 노드가 놀지 않음).
			// 요청량은 GPU 지분 × 최소 후보 노드 사양으로 상위(session)에서 산출해 넣는다.
			Resources: corev1.ResourceRequirements{
				Limits:   gpuLimits(s),
				Requests: cpuRequests(s),
			},
			VolumeMounts: mounts,
			WorkingDir:   homeMount, // ~ = /home/work (마운트 위치와 HOME 일치)
			Env:          append(homeEnv(homeMount), channelEnv(s.WebChannels)...),
			Ports:        channelPorts(s.WebChannels),
		}},
	}
	// 안정 UID 로 실행하면 NFS 볼륨 파일 소유가 물리 SSH(useradd -u UID)와 일관된다.
	// fsGroup 은 마운트 볼륨(emptyDir home, PVC)을 이 그룹 소유에 그룹쓰기로 만들어 비-root 프로세스가 쓸 수 있게 한다.
	if s.UID > 0 {
		uid := int64(s.UID)
		spec.SecurityContext = &corev1.PodSecurityContext{RunAsUser: &uid, RunAsGroup: &uid, FSGroup: &uid}
	}
	// 컨테이너 SSH 사이드카. 게이트웨이 SSH 프록시가 <iid>.<ns>.svc:22 로 도달한다.
	// 홈(/home/work)을 공유하고 게이트웨이 공개키만 신뢰한다. root 로 실행(22 바인드·useradd)하되
	// 로그인 계정 work 은 세션 UID 로 생성해 파일 소유를 세션 컨테이너와 일치시킨다.
	if s.SSHDImage != "" && s.UID > 0 {
		// 사용자 공개키 Secret 은 Optional 이다. 아직 키를 등록하지 않은 사용자도 세션은 떠야 하고,
		// 나중에 등록하면 Secret 이 생기면서 sshd 가 다음 접속부터 그 키를 신뢰한다.
		if s.UserKeysSecret != "" {
			optional := true
			spec.Volumes = append(spec.Volumes, corev1.Volume{
				Name: userKeysVolume,
				VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
					SecretName: s.UserKeysSecret,
					Optional:   &optional,
				}},
			})
		}
		spec.Containers = append(spec.Containers, sshdSidecar(s, mounts))
	}
	// 세션이 뜨기 전에 root 로 준비해야 하는 것들. 이미지를 재사용하므로 추가 풀은 없다.
	//   - 홈 안의 임시/캐시 디렉터리(TMPDIR 등이 가리키는 곳). 없으면 도구가 실패한다.
	//   - scratch(hostPath)는 kubelet 이 root 로 만들어 FSGroup 이 안 먹으므로 세션 UID 로 chown.
	if s.UID > 0 {
		root := int64(0)
		cmds := []string{homeDirsCmd(homeMount, s.UID)}
		initMounts := homeVolumeMounts(mounts)
		if s.ScratchHost != "" {
			cmds = append(cmds, fmt.Sprintf("chown %d:%d %s && chmod 700 %s", s.UID, s.UID, scratchMount, scratchMount))
			initMounts = append(initMounts, corev1.VolumeMount{Name: "scratch", MountPath: scratchMount})
		}
		spec.InitContainers = []corev1.Container{{
			Name:            "session-prep",
			Image:           s.Image,
			Command:         []string{"sh", "-c", strings.Join(cmds, " && ")},
			SecurityContext: &corev1.SecurityContext{RunAsUser: &root},
			VolumeMounts:    initMounts,
		}}
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: s.Name, Namespace: s.Namespace, Labels: s.Labels},
		Spec:       spec,
	}
}

// sshdSidecar는 컨테이너 SSH 접속용 사이드카 컨테이너를 만든다(홈 공유·게이트웨이 키 신뢰).
// 이미지 계약(entrypoint): env SESSION_UID(계정 UID)·SESSION_USER(계정명, 기본 work)·
// GATEWAY_PUBKEY(신뢰 공개키)·HOME_DIR 로 계정을 만들고 sshd 를 22 로 기동한다.
func sshdSidecar(s SessionSpec, homeMounts []corev1.VolumeMount) corev1.Container {
	root := int64(0)
	user := "work"
	// 홈 트리(/home/work 및 그 하위: ~/nfs·~/scratch·데이터셋 등)를 세션 컨테이너와 동일하게 마운트한다.
	// 그래야 SSH 로 보는 파일과 VSCode/Jupyter 로 보는 파일이 같다. 볼륨 이름은 상황마다 다르므로(공유홈=home,
	//   임시홈=vol-0) 이름이 아니라 마운트 '경로'로 고른다. serviceaccount 토큰 등은 제외.
	mounts := homeTreeMounts(homeMounts)
	// 사용자 등록 공개키(Secret)를 read-only 로 마운트한다. sshd_config 의 AuthorizedKeysFile 두 번째 경로다.
	if s.UserKeysSecret != "" {
		mounts = append(mounts, corev1.VolumeMount{Name: userKeysVolume, MountPath: userKeysMount, ReadOnly: true})
	}
	return corev1.Container{
		Name:  "sshd",
		Image: s.SSHDImage,
		// 사이드카 이미지는 노드에 미리 배급(ctr import)되는 운영 이미지라 태그가 latest 여도 당겨오지 않는다.
		// (기본 정책이면 :latest 가 Always 라 레지스트리에 없어 ImagePullBackOff 가 난다.)
		ImagePullPolicy: corev1.PullIfNotPresent,
		SecurityContext: &corev1.SecurityContext{RunAsUser: &root}, // root: 22 바인드·계정 생성(파드 UID 컨텍스트 오버라이드)
		Env: []corev1.EnvVar{
			{Name: "SESSION_UID", Value: itoa(s.UID)},
			{Name: "SESSION_USER", Value: user},
			{Name: "HOME_DIR", Value: homeMount},
			{Name: "GATEWAY_PUBKEY", Value: s.SSHDPubKey},
		},
		Ports:        []corev1.ContainerPort{{ContainerPort: 22, Name: "ssh"}},
		VolumeMounts: mounts,
	}
}

// homeTreeMounts는 홈과 그 하위 마운트만 고른다(serviceaccount 토큰 등 제외).
// 볼륨 이름은 상황마다 다르므로(공유홈=home, 임시홈=vol-0) 이름이 아니라 경로로 고른다.
func homeTreeMounts(all []corev1.VolumeMount) []corev1.VolumeMount {
	var out []corev1.VolumeMount
	for _, m := range all {
		if m.MountPath == homeMount || strings.HasPrefix(m.MountPath, homeMount+"/") {
			out = append(out, m)
		}
	}
	return out
}

// homeVolumeMounts는 홈 자체만 고른다. 준비 작업은 홈에만 쓰므로 하위 마운트(데이터셋 FUSE 등)를
// 끌어들일 이유가 없다.
func homeVolumeMounts(all []corev1.VolumeMount) []corev1.VolumeMount {
	var out []corev1.VolumeMount
	for _, m := range all {
		if m.MountPath == homeMount {
			out = append(out, m)
		}
	}
	return out
}

// itoa는 작은 정수 문자열 변환(외부 의존 없이).
func itoa(n int) string { return fmt.Sprintf("%d", n) }

// homeMount는 세션 홈 마운트 경로(/home/work). HOME/WorkingDir 를 여기로 맞춰 ~ 가 마운트 위치와 같게 한다.
const homeMount = "/home/work"

// 사용자 등록 공개키 Secret 의 볼륨명/마운트 경로. sshd_config 의 AuthorizedKeysFile 두 번째 경로와 일치해야 한다
// (/etc/giosk/ssh/authorized_keys). Secret 의 데이터 키 이름도 authorized_keys 여야 파일명이 맞는다.
const (
	userKeysVolume = "giosk-ssh-keys"
	userKeysMount  = "/etc/giosk/ssh"
)

// scratchMount는 노드로컬 스크래치 마운트 경로. 홈 하위(~/scratch)에 두어 접근 편의를 높인다
// (물리노드도 ~/scratch 심링크로 동일 관례). 홈 볼륨 위에 하위경로로 겹쳐 마운트된다.
const scratchMount = homeMount + "/scratch"

// homeEnv는 HOME 을 마운트 홈으로 강제하고(이미지 기본 HOME=/home/coder 등 무시), 임시 파일과
// 패키지 캐시도 홈 안으로 돌린다.
//
// 왜 돌리는가. 컨테이너 쓰기 레이어(overlay)는 노드 디스크를 그대로 쓰고, 컨테이너 단위로
// 크기를 강제할 방법이 런타임에 없다(containerd 의 overlayfs 스냅샷터에는 크기 옵션이 없다).
// 남은 수단은 ephemeral-storage 상한인데 그 집행은 파드 축출이라, 작업하던 세션이 통째로
// 죽는다. 반면 홈은 이미지 기반이라 넘기면 커널이 ENOSPC 로 막는다. 큰 쓰기는 대부분
// 임시 파일과 pip/conda/HuggingFace 캐시라, 그것들을 홈으로 보내면 축출이 아니라 정직한
// "용량 부족"으로 끝나고 사용자가 스스로 지울 수 있다. 홈 용량에서 세어지는 것도 맞다.
//
// 각 도구가 보는 표준 변수만 쓴다. 경로는 세션 시작 시 미리 만들어 둔다(prepareHome).
func homeEnv(home string) []corev1.EnvVar {
	cache := home + "/.cache"
	return []corev1.EnvVar{
		{Name: "HOME", Value: home},
		{Name: "TMPDIR", Value: home + "/.tmp"},
		{Name: "XDG_CACHE_HOME", Value: cache},
		{Name: "PIP_CACHE_DIR", Value: cache + "/pip"},
		{Name: "CONDA_PKGS_DIRS", Value: cache + "/conda"},
		{Name: "HF_HOME", Value: cache + "/huggingface"},
		{Name: "TORCH_HOME", Value: cache + "/torch"},
	}
}

// homeDirsCmd는 위 환경변수가 가리키는 디렉터리를 만드는 셸 명령이다. 없는 TMPDIR 을 주면
// 임시 파일을 쓰는 프로그램이 그냥 실패하므로 세션이 뜨기 전에 만들어 둬야 한다.
func homeDirsCmd(home string, uid int) string {
	dirs := home + "/.tmp " + home + "/.cache"
	return fmt.Sprintf("mkdir -p %s && chown %d:%d %s && chmod 700 %s", dirs, uid, uid, dirs, home+"/.tmp")
}

// channelEnv는 활성 웹 채널의 인증 시크릿을 컨테이너 EnvVar 로 변환한다(채널 순서 유지).
func channelEnv(chs []WebChannelSpec) []corev1.EnvVar {
	if len(chs) == 0 {
		return nil
	}
	out := make([]corev1.EnvVar, 0, len(chs))
	for _, c := range chs {
		if c.EnvKey != "" {
			out = append(out, corev1.EnvVar{Name: c.EnvKey, Value: c.Secret})
		}
	}
	return out
}

// channelPorts는 활성 웹 채널 포트를 컨테이너 포트로 노출한다.
func channelPorts(chs []WebChannelSpec) []corev1.ContainerPort {
	if len(chs) == 0 {
		return nil
	}
	out := make([]corev1.ContainerPort, 0, len(chs))
	for _, c := range chs {
		out = append(out, corev1.ContainerPort{ContainerPort: int32(c.Port), Name: c.Name})
	}
	return out
}

// podVolumes는 홈 PVC + 추가 볼륨을 Pod Volume/VolumeMount 로 매핑한다.
func podVolumes(s SessionSpec) ([]corev1.Volume, []corev1.VolumeMount) {
	var vols []corev1.Volume
	var mounts []corev1.VolumeMount
	add := func(name, pvc, path string, ro bool) {
		vols = append(vols, corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvc, ReadOnly: ro},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{Name: name, MountPath: path, ReadOnly: ro})
	}
	// /dev/shm 은 쿠버네티스 기본이 64MB tmpfs 라, PyTorch DataLoader(num_workers>0)가 공유메모리로
	// 텐서를 주고받다 꽉 차면 "Bus error / out of shared memory" 로 워커가 죽는다. Memory emptyDir 로
	// 넉넉히 준다. 사용량은 컨테이너 메모리 한도에 산입되므로, shm 단독 OOM 을 막게 메모리의 절반으로 캡한다.
	shmGi := s.MemGB / 2
	if shmGi < 1 {
		shmGi = 1
	}
	shmLimit := resource.MustParse(fmt.Sprintf("%dGi", shmGi))
	vols = append(vols, corev1.Volume{
		Name: "dshm",
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
			Medium: corev1.StorageMediumMemory, SizeLimit: &shmLimit,
		}},
	})
	mounts = append(mounts, corev1.VolumeMount{Name: "dshm", MountPath: "/dev/shm"})
	if s.HomePVC != "" {
		add("home", s.HomePVC, "/home/work", false)
	}
	for i, v := range s.Volumes {
		name := fmt.Sprintf("vol-%d", i)
		if v.EmptyDir { // 임시 워크스페이스는 emptyDir(파드가 사라지면 함께 사라진다)
			vols = append(vols, corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
			mounts = append(mounts, corev1.VolumeMount{Name: name, MountPath: v.MountPath})
			continue
		}
		if v.HostPath != "" { // 노드 로컬 캐시(hostPath) 마운트. RequireNode 로 해당 노드에 핀된다
			hp := corev1.HostPathDirectoryOrCreate
			vols = append(vols, corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: v.HostPath, Type: &hp}}})
			mounts = append(mounts, corev1.VolumeMount{Name: name, MountPath: v.MountPath, ReadOnly: v.ReadOnly})
			continue
		}
		add(name, v.PVCName, v.MountPath, v.ReadOnly)
	}
	// 노드로컬 스크래치는 계정 폴더만 hostPath 로 마운트한다(본인 폴더만 RWX, 없으면 생성). 언제든 삭제될 수 있다.
	if s.ScratchHost != "" {
		hp := corev1.HostPathDirectoryOrCreate
		vols = append(vols, corev1.Volume{
			Name:         "scratch",
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: s.ScratchHost, Type: &hp}},
		})
		mounts = append(mounts, corev1.VolumeMount{Name: "scratch", MountPath: scratchMount})
	}
	return vols, mounts
}

// nodeSelector는 GPU 타입이 지정되면 해당 라벨로 배치를 제한한다.
func nodeSelector(s SessionSpec, gpuTypeLabel string) map[string]string {
	if s.GpuType == "" || s.GpuMode == GpuModeCPU {
		return nil
	}
	return map[string]string{gpuTypeLabel: s.GpuType}
}

// shareModeReqs는 공유 전략에 맞는 노드로 배치를 가르는 affinity 조건이다.
//   - timeslice : 타임슬라이싱 노드에만(슬롯이 그 노드에만 광고됨).
//   - exclusive : 분할(HAMi)·타임슬라이싱 노드를 배제한다. 통 카드를 받아야 하므로.
//     (NotIn 은 라벨 없는 노드도 매칭하므로 무라벨 노드는 전용으로 계속 동작한다.)
//   - shared(HAMi) : HAMi 노드에만. HAMi 가 클러스터 전체에 설치되면 exclusive 노드도 gpumem/gpucores
//     를 광고해 무제약이면 공유 세션이 전용 노드에 떨어져 전용 가용성이 잘못 깎인다. In{hami} 로 강제한다.
func shareModeReqs(s SessionSpec) []corev1.NodeSelectorRequirement {
	switch s.GpuMode {
	case GpuModeTimeslice:
		return []corev1.NodeSelectorRequirement{{
			Key: ShareModeLabel, Operator: corev1.NodeSelectorOpIn, Values: []string{shareTimeslicing},
		}}
	case GpuModeExclusive:
		return []corev1.NodeSelectorRequirement{{
			Key: ShareModeLabel, Operator: corev1.NodeSelectorOpNotIn, Values: []string{shareTimeslicing, shareHami},
		}}
	case GpuModeShared:
		return []corev1.NodeSelectorRequirement{{
			Key: ShareModeLabel, Operator: corev1.NodeSelectorOpIn, Values: []string{shareHami},
		}}
	}
	return nil
}

// ShareModeAllows는 이 GPU 모드의 세션이 그 노드(공유 전략 라벨 값)에 배치될 수 있는지 판정한다.
// 위 shareModeReqs 가 파드에 굽는 required affinity 와 반드시 같은 답을 내야 한다.
//
// 스케줄 전에 노드를 골라 하드 핀하는 쪽(데이터셋 로컬 캐시)이 이 판정을 건너뛰면, 핀과 affinity 가
// 서로를 배제해 어떤 노드도 만족하지 못하는 영구 Pending 이 된다. 전용 세션을 HAMi 노드에 핀하는 경우다.
// 라벨이 없는 노드는 전용으로 취급한다(shareModeReqs 의 NotIn 이 무라벨 노드를 매칭하는 것과 같다).
func ShareModeAllows(gpuMode, nodeShareMode string) bool {
	switch gpuMode {
	case GpuModeTimeslice:
		return nodeShareMode == shareTimeslicing
	case GpuModeExclusive:
		return nodeShareMode != shareTimeslicing && nodeShareMode != shareHami
	case GpuModeShared:
		return nodeShareMode == shareHami
	}
	return true // CPU 전용이나 모드 미지정은 배치 제약이 없다
}

// nodeAffinity는 이미지 캐시 노드 소프트 선호(prefer) + 데이터셋 로컬캐시 하드 핀(require)
// + 공유 전략 제약(reqs)을 구성한다. required 조건들은 한 term 안에서 AND 로 묶인다.
//   - require 가 있으면 해당 노드(hostname)로 required 제약(데이터셋 hostPath 가 그 노드에만 있으므로 필수).
//   - reqs 는 공유 전략(타임슬라이싱 In / 전용 NotIn) 제약.
//   - prefer 는 weight 100 소프트 선호(이미지 캐시 빠른 시작). 전부 없으면 nil.
func nodeAffinity(prefer []string, require string, reqs []corev1.NodeSelectorRequirement) *corev1.Affinity {
	if len(prefer) == 0 && require == "" && len(reqs) == 0 {
		return nil
	}
	na := &corev1.NodeAffinity{}
	required := append([]corev1.NodeSelectorRequirement{}, reqs...)
	if require != "" {
		required = append(required, corev1.NodeSelectorRequirement{
			Key: "kubernetes.io/hostname", Operator: corev1.NodeSelectorOpIn, Values: []string{require},
		})
	}
	if len(required) > 0 {
		na.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: required}},
		}
	}
	if len(prefer) > 0 {
		na.PreferredDuringSchedulingIgnoredDuringExecution = []corev1.PreferredSchedulingTerm{{
			Weight: 100,
			Preference: corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key: "kubernetes.io/hostname", Operator: corev1.NodeSelectorOpIn, Values: prefer,
			}}},
		}}
	}
	return &corev1.Affinity{NodeAffinity: na}
}

// gpuLimits는 모드별 GPU 확장 리소스 limit + (정책이 지정한 경우에만) ephemeral-storage 상한을 구성한다.
func gpuLimits(s SessionSpec) corev1.ResourceList {
	limits := corev1.ResourceList{}
	switch s.GpuMode {
	case GpuModeExclusive:
		limits[resGPU] = *resource.NewQuantity(int64(max1(s.GpuCount)), resource.DecimalSI)
	case GpuModeTimeslice:
		// 타임슬라이싱 노드는 device plugin 이 GPU 를 replica 만큼 광고한다.
		// 슬롯 1개 = nvidia.com/gpu:1 요청(전용과 요청 형태가 같으므로 배치는 라벨로 가른다).
		limits[resGPU] = *resource.NewQuantity(1, resource.DecimalSI)
	case GpuModeShared:
		if s.VramMB > 0 {
			limits[resGPUMem] = *resource.NewQuantity(int64(s.VramMB), resource.DecimalSI)
		}
		if s.CorePercent > 0 {
			limits[resGPUCores] = *resource.NewQuantity(int64(s.CorePercent), resource.DecimalSI)
		}
	}
	// ephemeral-storage 상한(컨테이너 쓰기 레이어 + emptyDir + 로그).
	//
	// 컨테이너 쓰기 레이어는 노드 디스크를 그대로 쓴다. containerd 의 overlayfs 스냅샷터에는
	// 컨테이너별 크기 옵션이 없어서 커널이 막아 주는 하드 리밋을 걸 수단이 없다. 남은 것은 이
	// limit 뿐이고, 집행은 kubelet 의 축출(세션 파드 강제 종료)이다. 그래서 큰 쓰기가 여기로
	// 오지 않도록 임시 파일과 캐시를 홈으로 돌려 두었다(homeEnv). 그 위에서 이 값은 폭주를
	// 막는 마지막 방어선이다. 0(미설정)이면 걸지 않는다.
	if s.EphemeralGiB > 0 {
		limits[corev1.ResourceEphemeralStorage] = resource.MustParse(fmt.Sprintf("%dGi", s.EphemeralGiB))
	}
	// 메모리 상한 = request × MemBurst. CPU 와 달리 메모리는 압축 불가라 limit 이 없으면
	// 한 세션이 노드 RAM 을 다 먹으면 kubelet 이 "다른 사용자의" 파드를 축출한다.
	// 버스트의 대가를 남이 치르는 구조. 배수를 주어 버스트는 살리되 연쇄는 끊는다.
	// (CPU 는 압축 가능해 경합 시 느려질 뿐이므로 지금처럼 limit 없이 둔다.)
	if s.MemGB > 0 && s.MemBurst > 1 {
		limits[corev1.ResourceMemory] = resource.MustParse(fmt.Sprintf("%dGi", s.MemGB*s.MemBurst))
	}
	if len(limits) == 0 {
		return nil
	}
	return limits
}

// cpuRequests는 CPU/메모리 요청을 구성한다.
func cpuRequests(s SessionSpec) corev1.ResourceList {
	req := corev1.ResourceList{}
	if s.CPUCores > 0 {
		req[corev1.ResourceCPU] = *resource.NewQuantity(int64(s.CPUCores), resource.DecimalSI)
	}
	if s.MemGB > 0 {
		req[corev1.ResourceMemory] = resource.MustParse(fmt.Sprintf("%dGi", s.MemGB))
	}
	// ephemeral-storage 는 request 도 같이 건다. limit 만 걸면 스케줄러가 이 소비를 모르고
	// 노드에 세션을 계속 얹어, 합계가 노드 디스크를 넘긴 뒤 kubelet 이 축출로 뒷수습한다.
	if s.EphemeralGiB > 0 {
		req[corev1.ResourceEphemeralStorage] = resource.MustParse(fmt.Sprintf("%dGi", s.EphemeralGiB))
	}
	if len(req) == 0 {
		return nil
	}
	return req
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
