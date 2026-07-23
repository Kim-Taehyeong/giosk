// 사용 가이드 문서 (Markdown 형식). 가이드 탭에 표시. ko/en 둘 다 보유.
// 본문 이미지는 실제 콘솔 화면 캡처(public/guide/*.png)를 ![캡션](/guide/<파일>.png) 로 참조한다.
export const GUIDES = [
  {
    id: 'start',
    title: { ko: '처음 시작하기', en: 'Getting Started' },
    body: {
      ko: `# 처음 시작하기

Giosk Console는 기관 구성원이 GPU 자원을 손쉽게 빌려 쓰도록 만든 내부 클라우드 플랫폼입니다. 이 문서는 처음 로그인한 사용자가 화면을 익히고, 첫 세션을 띄워 접속하기까지의 전 과정을 안내합니다. 대부분의 경우 로그인부터 노트북 접속까지 5분이면 충분합니다.

![로그인 직후 만나는 대시보드 — 빠른 시작 카드와 크레딧·세션 현황](/guide/dash.png)

## 1. 대시보드 한눈에 보기

로그인하면 가장 먼저 \`대시보드\`가 열립니다. 상단에는 현재 **조직**과 **그룹**, 보유 **크레딧**이 표시되고, 가운데 \`빠른 시작\` 카드로 GPU 세션·CPU 작업·새 볼륨을 한 번에 시작할 수 있습니다. 아래에는 크레딧 잔액, 활성 세션 수, 이번 달 GPU 사용시간, 리전 가용성이 한눈에 정리됩니다.

- 왼쪽 사이드바: \`세션\` \`데이터/볼륨\` \`데이터셋\` \`지갑/크레딧\` \`그룹 가입 신청\` \`알림 센터\` \`내 정보·설정\`.
- 상단 막대: 조직·그룹 선택기와 크레딧 잔액, 언어/테마 전환.

## 2. 내 정보와 소속 확인

\`내 정보·설정\`에서 본인의 프로필과 **소속(조직·그룹·그룹 역할)**을 확인하세요. 그룹에 소속되어 있으면 세션 비용을 그룹과 함께 관리할 수 있고, 소속이 비어 있다면 \`그룹 가입 신청\`에서 그룹에 가입을 요청할 수 있습니다.

![내 정보·설정 — 프로필과 소속(조직·그룹·역할), SSH 공개키 등록](/guide/account.png)

### SSH 공개키 등록

터미널로 세션에 접속하려면 같은 화면 아래의 \`SSH 공개키\` 칸에 공개키를 등록하세요. 키가 없다면 아래 명령으로 새로 만든 뒤 \`.pub\` 파일의 내용을 붙여 넣고 저장합니다. 공개키는 세션이 생성되는 순간 컨테이너에 자동으로 주입됩니다.

\`\`\`bash
ssh-keygen -t ed25519 -C "you@giosk.io"
cat ~/.ssh/id_ed25519.pub
\`\`\`

## 3. 첫 세션 만들기

\`빠른 시작 → GPU 세션 생성\` 카드를 누르거나 \`세션 → + 새 세션\`으로 이동하면 단계별 마법사가 열립니다. 유형을 고르고, 자원과 이미지를 선택한 뒤, 필요하면 볼륨을 연결하고 마지막으로 검토 후 시작하면 됩니다. 자세한 절차는 \`세션 만들기\` 가이드를 참고하세요.

## 자주 묻는 질문

- **세션을 꺼도 데이터가 남나요?** \`~/nfs\` 영속 홈과 연결한 볼륨에 저장한 내용은 유지됩니다. 컨테이너 임시 영역은 사라지니 중요한 산출물은 볼륨에 저장하세요.
- **크레딧이 떨어지면?** 실행 중인 세션은 잔액 소진 시 자동으로 정지됩니다. 미리 \`지갑 → 충전 요청\`으로 보충하세요.
- **유휴 상태로 두면?** 일정 시간(기본 30분) 활동이 없으면 세션이 자동 반환되어 불필요한 소모를 막습니다.`,
      en: `# Getting Started

Giosk Console is an internal platform that lets members of your organization borrow GPU resources with ease. This document walks a brand-new user through the screens and everything from sign-in to launching and connecting to a first session. In most cases five minutes is enough.

![The dashboard you land on after login — quick-start cards and credit/session status](/guide/dash.png)

## 1. The dashboard at a glance

After login the \`Dashboard\` opens first. The top bar shows your current **organization**, **group**, and **credit** balance, and the \`Quick start\` cards launch a GPU session, a CPU job, or a new volume in one click. Below, you can see your credit balance, active sessions, GPU hours this month, and regional availability.

- Left sidebar: \`Sessions\`, \`Data/Volumes\`, \`Datasets\`, \`Wallet/Credits\`, \`Join a group\`, \`Alerts\`, \`My settings\`.
- Top bar: org/group selector, credit balance, language and theme toggles.

## 2. Check your profile and membership

Open \`My settings\` to review your profile and **membership (organization, group, group role)**. If you belong to a group you can manage session cost together with it; if membership is empty, request to join a group under \`Join a group\`.

![My settings — profile, membership (org/group/role), and SSH public key](/guide/account.png)

### Register your SSH key

To connect over a terminal, paste your public key into the \`SSH public key\` box lower on the same screen. If you do not have one, generate it and paste the contents of the \`.pub\` file. The key is injected into the container the moment a session is created.

\`\`\`bash
ssh-keygen -t ed25519 -C "you@giosk.io"
cat ~/.ssh/id_ed25519.pub
\`\`\`

## 3. Create your first session

Click \`Quick start → Create GPU session\` or go to \`Sessions → + New session\` to open the step-by-step wizard. Pick a type, choose resources and an image, optionally attach volumes, then review and start. See the \`Creating a Session\` guide for the full walkthrough.

## FAQ

- **Does data survive a stop?** Anything saved to your \`~/nfs\` persistent home or an attached volume is kept. The container scratch area is wiped — store important outputs on a volume.
- **What if I run out of credits?** Running sessions auto-stop when the balance is exhausted. Top up ahead of time via \`Wallet → Request top-up\`.
- **What if it sits idle?** After a period of inactivity (30 min by default) the session is reclaimed automatically to avoid wasted consumption.`,
    },
  },
  {
    id: 'session',
    title: { ko: '세션 만들기', en: 'Creating a Session' },
    body: {
      ko: `# 세션 만들기

세션은 GPU(또는 CPU)와 개발 환경이 묶인 작업 공간입니다. \`세션 → + 새 세션\`을 누르면 5단계 마법사가 열립니다: **유형 → 오퍼링(자원) → 이미지 → 볼륨 연결 → 검토·시작**. 각 단계는 좌측에 진행 상태로 표시됩니다.

## 1단계. 워크로드 유형

먼저 GPU를 어떻게 쓸지 고릅니다. 공유 GPU는 하나의 카드를 나눠 쓰고, 전용 GPU는 카드를 통째로 점유하며, CPU는 무료입니다. 자세한 차이는 \`공유 GPU vs 전용 GPU\` 가이드를 참고하세요.

![1단계 — 공유 GPU / 전용 GPU / CPU(무료) 중 선택](/guide/sess-type.png)

## 2단계. 오퍼링(자원) 선택

전용 GPU를 골랐다면 클러스터의 **GPU 모델**(예: Tesla-T4, A100, RTX-4090)과 장수를 고릅니다. 각 카드에는 시간당 단가와 **가용 현황**이 함께 표시되어, 지금 바로 받을 수 있는지 확인할 수 있습니다. 공유 GPU는 미리 정의된 오퍼링을 고르거나 고급 모드에서 VRAM·코어를 직접 조정합니다.

![2단계 — GPU 모델·장수 선택, 가용 현황과 예상 비용 확인](/guide/sess-spec-picked.png)

## 3단계. 이미지 선택

프레임워크가 미리 설치된 환경을 고릅니다. **접속 방식(VSCode/Jupyter/SSH)은 이미지가 결정**합니다. 예를 들어 code-server 이미지는 브라우저 VSCode와 SSH를, Jupyter 이미지는 노트북을 제공합니다.

![3단계 — 이미지가 연결 방식(VSCode·Jupyter·SSH)을 결정](/guide/sess-image.png)

## 4단계. 볼륨 연결 (선택)

영구 저장이나 공유 데이터가 필요하면 \`볼륨 연결\` 단계에서 볼륨을 골라 마운트 경로를 지정합니다. 승인된 데이터셋은 자동으로 \`~/datasets\` 에 마운트되므로 여기서 따로 고를 필요가 없습니다. 볼륨이 없다면 비워 두고 넘어가도 됩니다.

![4단계 — 영구 볼륨 연결(선택). 데이터셋은 자동 마운트](/guide/sess-volume.png)

## 5단계. 검토 후 시작

마지막으로 세션 이름과 SSH 공개키(선택)를 입력하고 설정을 검토합니다. 유형·자원·이미지·연결 방식·예상 비용·유휴 반환 시간이 요약으로 표시됩니다. \`세션 시작\`을 누르면 비용이 먼저 예약되고 잠시 후 상태가 \`실행\`으로 바뀝니다.

![5단계 — 설정 요약과 예상 비용 확인 후 시작](/guide/sess-review.png)

## 세션에 접속하기

세션 목록에서 상태가 \`실행\`이 되면 \`연결\` 열의 버튼으로 바로 접속할 수 있습니다. VSCode는 브라우저 IDE로 열리고, SSH는 등록한 공개키로 터미널에 연결됩니다.

![세션 목록 — 실행 상태, VSCode 연결 버튼, 정지·자세히](/guide/sess-list.png)

\`자세히\`를 누르면 세션 ID, 배치된 노드·GPU, 실시간 GPU 사용률과 VRAM, 활동 기록을 볼 수 있습니다.

![세션 상세 — 노드·GPU 사용률·VRAM·활동 기록](/guide/sess-detail.png)

> 작업이 끝나면 \`정지\`로 세션을 반환해 크레딧을 아끼세요. 유휴 30분이 지나면 자동으로 반환됩니다.`,
      en: `# Creating a Session

A session is a workspace that bundles a GPU (or CPU) with a development environment. Click \`Sessions → + New session\` to open a five-step wizard: **type → offering (resources) → image → attach volumes → review & start**. Progress is shown on the left.

## Step 1. Workload type

First decide how to use the GPU. A shared GPU splits one card, a dedicated GPU occupies a whole card, and CPU is free. See the \`Shared vs Dedicated GPU\` guide for details.

![Step 1 — choose Shared GPU / Dedicated GPU / CPU (free)](/guide/sess-type.png)

## Step 2. Pick an offering (resources)

For a dedicated GPU, pick the cluster **GPU model** (e.g. Tesla-T4, A100, RTX-4090) and count. Each card shows the hourly price and **availability**, so you can tell whether it can start right now. For a shared GPU, pick a predefined offering or tune VRAM and cores in advanced mode.

![Step 2 — choose GPU model and count; see availability and estimated cost](/guide/sess-spec-picked.png)

## Step 3. Choose an image

Pick a preinstalled environment. **The image decides the connection method (VSCode/Jupyter/SSH).** For example, a code-server image offers browser VSCode and SSH, while a Jupyter image offers a notebook.

![Step 3 — the image decides the connection method (VSCode/Jupyter/SSH)](/guide/sess-image.png)

## Step 4. Attach volumes (optional)

If you need persistent or shared data, pick volumes and set a mount path in the \`Attach volumes\` step. Approved datasets are mounted automatically under \`~/datasets\`, so you do not select them here. If you have no volumes, just continue.

![Step 4 — attach persistent volumes (optional). Datasets mount automatically](/guide/sess-volume.png)

## Step 5. Review and start

Finally enter a session name and an optional SSH key, then review the configuration. Type, resources, image, connection method, estimated cost, and idle-reclaim time are summarized. Clicking \`Start\` holds the cost first, then the status switches to \`running\`.

![Step 5 — review the summary and estimated cost, then start](/guide/sess-review.png)

## Connecting to the session

Once the status is \`running\`, use the button in the \`Connect\` column to jump in. VSCode opens a browser IDE, and SSH connects to a terminal with your registered key.

![Session list — running status, VSCode connect button, stop/details](/guide/sess-list.png)

Click \`Details\` to see the session ID, scheduled node and GPU, live GPU utilization and VRAM, and the activity log.

![Session details — node, GPU utilization, VRAM, activity log](/guide/sess-detail.png)

> When you are done, \`Stop\` the session to save credits. It is reclaimed automatically after 30 minutes of idleness.`,
    },
  },
  {
    id: 'gpu',
    title: { ko: '공유 GPU vs 전용 GPU', en: 'Shared vs Dedicated GPU' },
    body: {
      ko: `# 공유 GPU vs 전용 GPU

워크로드의 성격에 따라 GPU를 어떻게 빌릴지 결정하는 것은 비용과 성능 모두에 큰 영향을 줍니다. 세션 마법사 1단계에서 세 가지 방식 중 하나를 고릅니다.

![세션 1단계 — 공유 GPU / 전용 GPU / CPU(무료)](/guide/sess-type.png)

## 공유 GPU (HAMi 분할)

하나의 물리 GPU를 여러 사용자가 \`VRAM\`과 \`연산 코어(%)\` 단위로 나눠 씁니다. 추론 서비스, 노트북 실습, 가벼운 학습처럼 GPU를 100% 점유하지 않는 작업에 경제적입니다. 같은 카드를 공유하므로 다른 사용자의 부하에 따라 약간의 성능 변동이 있을 수 있습니다.

- 적은 VRAM으로 충분한 추론·소형 모델에 적합합니다.
- 비용은 요청한 VRAM과 코어 비율에 비례합니다.
- 고급 모드에서 슬라이더로 필요한 만큼만 세밀하게 요청할 수 있습니다.

## 전용 GPU

GPU 카드를 통째로 점유합니다. 다른 작업의 간섭이 전혀 없어 대형 모델 학습, 벤치마크, 재현이 중요한 실험에 적합합니다. 여러 장을 묶어 분산 학습도 가능합니다. 마법사 2단계에서 모델별 가용 현황과 시간당 단가를 보고 고를 수 있습니다.

![전용 GPU — 모델·장수 선택, 모델별 가용 현황과 단가](/guide/sess-spec-picked.png)

- 성능 간섭이 없어 결과가 안정적입니다.
- 비용은 점유한 GPU 장수에 비례합니다.
- 수요가 많을 때는 가용 대수가 0이 되어 대기하거나 알림을 신청할 수 있습니다.

## CPU (무료)

GPU 없이 CPU 풀에서만 실행합니다. 데이터셋 다운로드, 전처리, 압축 해제처럼 GPU가 필요 없는 준비 작업에 사용하세요. 크레딧이 **0**으로 완전히 무료이며, 본 학습 전에 데이터를 미리 준비해 두면 GPU 시간을 아낄 수 있습니다.

## 어떻게 고를까

| 상황 | 추천 |
| --- | --- |
| 추론·노트북·가벼운 실습 | 공유 GPU |
| 대형 학습·벤치마크 | 전용 GPU |
| 데이터 준비·전처리 | CPU(무료) |

정리하면, 평소에는 공유 GPU로 비용을 아끼고, 본격적인 학습이 필요할 때만 전용 GPU를 점유하는 방식이 가장 효율적입니다.`,
      en: `# Shared vs Dedicated GPU

How you borrow a GPU has a big impact on both cost and performance. In step 1 of the session wizard you pick one of three modes.

![Wizard step 1 — Shared GPU / Dedicated GPU / CPU (free)](/guide/sess-type.png)

## Shared GPU (HAMi)

Multiple users split one physical GPU by \`VRAM\` and \`compute cores (%)\`. It is economical for inference services, notebook practice, and light training that does not fully occupy a GPU. Because the card is shared, performance may vary slightly with other users' load.

- Great for inference and small models that fit in limited VRAM.
- Cost is proportional to the requested VRAM and core ratio.
- Advanced mode lets you request exactly what you need with sliders.

## Dedicated GPU

Occupy a whole GPU card. With zero interference it suits large-model training, benchmarks, and reproducibility-critical experiments. You can also bundle several cards for distributed training. In step 2 you pick by per-model availability and hourly price.

![Dedicated GPU — pick model and count; per-model availability and price](/guide/sess-spec-picked.png)

- Stable results thanks to no interference.
- Cost is proportional to the number of GPUs held.
- During peak demand availability can hit zero — queue or request an alert.

## CPU (free)

Runs on the CPU pool with no GPU. Use it for prep work that needs no GPU — dataset downloads, preprocessing, decompression. It costs **0** credits, and preparing data ahead saves GPU time later.

## How to choose

| Situation | Recommended |
| --- | --- |
| Inference, notebooks, light practice | Shared GPU |
| Large training, benchmarks | Dedicated GPU |
| Data prep, preprocessing | CPU (free) |

In short, save cost with shared GPUs day to day and occupy a dedicated GPU only when you truly need full training throughput.`,
    },
  },
  {
    id: 'volume',
    title: { ko: '데이터 / 볼륨 사용법', en: 'Data / Volumes' },
    body: {
      ko: `# 데이터 / 볼륨 사용법

세션은 일시적이지만 데이터는 오래 유지되어야 합니다. 볼륨은 세션이 사라져도 남는 영구 저장 공간이며, 동료와 데이터를 공유하는 통로이기도 합니다.

## 볼륨 만들기

\`데이터 / 볼륨 → + 새 볼륨\`을 누르고 이름과 용량(GB)을 입력하면 됩니다. 화면 위쪽에 본인에게 할당된 **볼륨 용량 한도**와 남은 용량이 표시되어, 한도 안에서만 생성할 수 있습니다. 볼륨은 NFS에 생성되어 어느 노드의 세션에서나 마운트됩니다.

![새 볼륨 생성 — 이름·용량 입력, 용량 한도 확인](/guide/vol-create.png)

생성한 볼륨은 \`내 볼륨\` 목록에 나타나며, 사용량과 공유 현황을 확인하고 \`자세히\`로 펼쳐 볼 수 있습니다.

![내 볼륨 목록 — 사용량·공유 상태·작업(공유/삭제)](/guide/vol-list.png)

## 세션에 볼륨 연결하기

세션 생성 마법사의 \`볼륨 연결\` 단계에서 사용할 볼륨을 고르고 마운트 경로를 지정하면 됩니다. 기본 경로는 \`/data/<이름>\` 이며 필요에 따라 바꿀 수 있습니다.

\`\`\`bash
# 마운트된 볼륨 확인
ls -al /data/exp-checkpoints
\`\`\`

## 볼륨 공유하기

\`내 볼륨 → 공유\`에서 다른 사용자나 그룹 전체에 볼륨을 공유할 수 있습니다. 공유 대상(특정 사용자/그룹 전체)과 권한(읽기 전용 \`ro\` 또는 읽기·쓰기 \`rw\`)을 고르면 됩니다.

![볼륨 공유 — 대상과 권한(ro/rw) 선택](/guide/vol-share.png)

- **특정 사용자 공유**: 지정한 사용자만 해당 볼륨을 마운트할 수 있습니다.
- **그룹 전체 공유**: 그룹의 모든 멤버가 자동으로 마운트 권한을 얻습니다.

데이터처럼 변경되면 안 되는 자료는 \`ro\`로 공유해 실수로 덮어쓰는 일을 막는 것이 좋습니다.

## 볼륨의 종류

- **개인 볼륨**: 내가 만든 영구 저장 공간. 세션을 지워도 유지됩니다.
- **공유받은 볼륨**: 다른 사용자·그룹이 나에게 공유한 볼륨. 권한에 따라 읽기 또는 읽기·쓰기.
- **데이터셋**: 여러 사람이 읽기 전용으로 쓰는 대용량 공개 데이터. 세션에 자동 마운트됩니다(\`데이터셋\` 가이드 참고).`,
      en: `# Data / Volumes

Sessions are ephemeral, but data must persist. Volumes are durable storage that survives a session, and the channel through which you share data with colleagues.

## Creating a volume

Click \`Data / Volumes → + New volume\` and enter a name and capacity (GB). The top of the dialog shows your **volume quota** and remaining space, so you can only create within the limit. Volumes are created on NFS and mount from a session on any node.

![New volume — enter name and capacity; see your quota](/guide/vol-create.png)

The volume then appears in your \`My volumes\` list, where you can check usage and sharing status and expand \`Details\`.

![My volumes — usage, sharing status, actions (share/delete)](/guide/vol-list.png)

## Attaching volumes to a session

In the \`Attach volumes\` step of the wizard, pick the volumes and set a mount path. The default path is \`/data/<name>\`, which you can change.

\`\`\`bash
# inspect a mounted volume
ls -al /data/exp-checkpoints
\`\`\`

## Sharing a volume

Use \`My volumes → Share\` to share with another user or an entire group. Choose the target (a specific user or a whole group) and permission (read-only \`ro\` or read/write \`rw\`).

![Share a volume — choose target and permission (ro/rw)](/guide/vol-share.png)

- **Share with a user**: only that user can mount the volume.
- **Share with a group**: every member of the group automatically gets mount access.

For data that must not change, share as \`ro\` to prevent accidental overwrites.

## Volume types

- **Personal volume**: durable storage you created. Kept even after a session is deleted.
- **Shared with me**: volumes another user or group shared with you — read or read/write per permission.
- **Datasets**: large read-only public data many people use; mounted into sessions automatically (see the \`Datasets\` guide).`,
    },
  },
  {
    id: 'dataset',
    title: { ko: '데이터셋', en: 'Datasets' },
    body: {
      ko: `# 데이터셋

데이터셋은 여러 사람이 **읽기 전용**으로 함께 쓰는 대용량 공개 데이터입니다. 한 번 등록해 두면 모두가 사용할 수 있고, 각자 개인 용량을 쓰지 않아도 됩니다.

## 전역 데이터셋 둘러보기

\`데이터셋\` 화면의 **전역 데이터셋** 목록에서 현재 사용 가능한 데이터셋과 크기·클래스·캐시 상태를 볼 수 있습니다. 데이터셋은 NFS에 저장되어, 세션을 만들면 자동으로 \`~/datasets\` 아래에 마운트됩니다 — 따로 다운로드할 필요가 없습니다.

![전역 데이터셋 목록 — 이름·클래스·용량·캐시 노드](/guide/ds-list.png)

\`\`\`bash
# 세션 안에서 마운트된 데이터셋 확인
ls ~/datasets/slow/cifar-10
\`\`\`

- \`~/datasets/slow/<이름>\`: NFS에서 직접 읽는 기본 마운트.
- \`~/datasets/fast/<이름>\`: 노드 로컬 캐시로 복사된 빠른 마운트(관리자가 캐시를 배치한 경우).

## 새 데이터셋 등록하기

목록에 없는 데이터를 추가하려면 \`데이터셋 등록\`을 누릅니다. 이름과 사이즈 클래스를 고르고, 내려받을 \`Wget 주소\`(zip 권장)를 입력해 신청합니다. 관리자가 승인하면 시스템이 해당 주소에서 데이터를 내려받아 NFS에 적재하고, 이후 모든 사용자가 쓸 수 있게 됩니다.

![데이터셋 등록 — 이름·사이즈 클래스·다운로드 주소 입력](/guide/ds-register.png)

신청한 항목은 같은 화면의 **내 등록 요청**에서 \`대기 → 승인\` 상태로 추적됩니다. 승인 후 적재가 끝나면 전역 데이터셋 목록에 나타납니다.

> 큰 데이터셋은 무료 CPU 세션으로 받아 정리한 뒤 등록하면 GPU 시간을 아낄 수 있습니다. 변경이 잦은 개인 작업 데이터는 데이터셋 대신 \`볼륨\`을 사용하세요.`,
      en: `# Datasets

A dataset is large public data that many people use **read-only**. Register it once and everyone can use it without spending personal capacity.

## Browse global datasets

The **Global datasets** list on the \`Datasets\` screen shows what is available along with size, class, and cache status. Datasets live on NFS and mount automatically under \`~/datasets\` when you create a session — no separate download needed.

![Global dataset list — name, class, size, cache nodes](/guide/ds-list.png)

\`\`\`bash
# inspect a mounted dataset inside a session
ls ~/datasets/slow/cifar-10
\`\`\`

- \`~/datasets/slow/<name>\`: the default mount, read directly from NFS.
- \`~/datasets/fast/<name>\`: a faster mount copied to a node-local cache (when an admin places a cache).

## Register a new dataset

To add data that is not listed, click \`Register dataset\`. Choose a name and size class, and enter a \`Wget URL\` to download from (zip recommended). Once an admin approves, the system downloads the data into NFS, after which all users can use it.

![Register a dataset — name, size class, and download URL](/guide/ds-register.png)

Your request is tracked under **My requests** on the same screen, moving from \`pending → approved\`. After ingestion completes it appears in the global dataset list.

> For large datasets, download and tidy them on a free CPU session before registering to save GPU time. For frequently changing personal data, use a \`volume\` instead of a dataset.`,
    },
  },
  {
    id: 'credit',
    title: { ko: '크레딧 / 지갑', en: 'Credits / Wallet' },
    body: {
      ko: `# 크레딧 / 지갑

크레딧은 Giosk 내부에서 자원 사용량을 측정하는 단위입니다. 실제 화폐 결제는 일어나지 않으며, 공정한 자원 배분과 사용량 추적을 위한 장치입니다.

## 지갑 화면 보기

\`지갑 / 크레딧\` 화면에서 **가용 잔액**, 현재 소모 속도로 보는 **소진 예상**, **이번 달 사용량**을 한눈에 볼 수 있습니다. 아래에는 최근 30주 소모 추이(잔디)와 달력, 크레딧 내역, 세션별 소모가 정리됩니다.

![지갑 / 크레딧 — 잔액·소진 예상·소모 추이](/guide/wallet.png)

## 크레딧 생애주기

세션을 시작하면 예상 비용이 먼저 잠깁니다. 실행되는 동안 실제 사용량만큼 차감되고, 세션을 종료하면 남은 예약분이 정산됩니다.

- **예약(hold)**: 세션 시작 시 예상 비용만큼 잔액을 잠급니다.
- **소모(consume)**: 실행 시간에 비례해 실제로 차감합니다.
- **정산(settle)**: 종료 시 사용분을 확정하고 남은 예약을 반환합니다.
- **환불(refund)**: 시작 직후 실패한 세션 등은 예약분이 되돌아옵니다.

## 충전 요청하기

잔액이 부족하면 \`충전 요청\`을 눌러 금액과 사유를 적어 신청합니다. 그룹 관리자(또는 운영자)가 검토 후 승인하면 잔액에 즉시 반영됩니다. 실제 결제가 아니므로, 사유를 구체적으로 적을수록 승인이 빠릅니다.

![크레딧 충전 요청 — 금액과 사유 입력](/guide/wallet-topup.png)

## 절약 팁

- 데이터 준비는 무료 CPU 세션으로 끝내고 GPU 시간을 아끼세요.
- 잔액이 적어지면 \`알림 센터\`에 \`크레딧 잔액 ≤ N\` 규칙을 만들어 미리 경고를 받을 수 있습니다.
- 공유 GPU로 충분한 작업에 전용 GPU를 쓰지 않도록 주의하고, 유휴 세션은 바로 정지하세요.`,
      en: `# Credits / Wallet

Credits are the unit Giosk uses to measure resource usage. No real money changes hands — they exist for fair allocation and usage tracking.

## The wallet screen

The \`Wallet / Credits\` screen shows your **available balance**, an **ETA to depletion** at the current burn rate, and **usage this month** at a glance. Below are a 30-week burn heatmap and calendar, the credit ledger, and per-session spend.

![Wallet / Credits — balance, depletion ETA, burn trend](/guide/wallet.png)

## Credit lifecycle

When a session starts, the estimated cost is locked first. While it runs, only actual usage is deducted, and on stop the remaining hold is settled.

- **Hold**: locks the estimated cost at start.
- **Consume**: deducts in proportion to runtime.
- **Settle**: finalizes usage on stop and returns the remaining hold.
- **Refund**: returns the hold for sessions that fail right after start.

## Requesting a top-up

When low, click \`Request top-up\` and enter an amount and reason. Once a group admin (or operator) approves, it reflects immediately. Since there is no real payment, a specific reason speeds approval.

![Request a credit top-up — enter amount and reason](/guide/wallet-topup.png)

## Saving tips

- Finish data prep on free CPU sessions to save GPU time.
- When your balance gets low, create a \`credit balance ≤ N\` rule in \`Alerts\` to get warned early.
- Avoid dedicated GPUs for work a shared GPU handles, and stop idle sessions promptly.`,
    },
  },
  {
    id: 'org',
    title: { ko: '조직과 그룹의 차이', en: 'Organizations & Groups' },
    body: {
      ko: `# 조직과 그룹의 차이

Giosk의 소속 구조는 **조직(Organization)**과 **그룹(Group)** 두 계층으로 이루어집니다. 둘을 구분하면 크레딧이 어디서 나오고 자원 정책이 어떻게 적용되는지 이해할 수 있습니다.

## 조직(Organization)

조직은 단과대·연구센터처럼 **최상위 단위**입니다. 전체 크레딧 풀과 정책의 출발점이며, 그 아래에 여러 그룹을 둡니다. 예를 들어 \`AI응용연구센터\` 조직 안에 \`비전 연구실\`, \`자연어처리 연구실\` 같은 그룹이 속합니다.

## 그룹(Group)

그룹은 실제 예산과 자원이 적용되는 **연구실·프로젝트 단위**입니다. 세션 비용은 그룹 지갑과 함께 관리되고, 그룹 관리자가 멤버와 예산을 운영합니다. 그룹 안에서 내 역할은 다음과 같습니다.

- **멤버(member)**: 세션·볼륨을 만들고 자원을 사용합니다.
- **그룹 관리자(project admin)**: 멤버·예산·가입 신청을 관리합니다.

내 소속과 그룹 역할은 \`내 정보·설정\`의 **소속** 카드에서 확인할 수 있습니다.

![내 정보·설정 — 조직·그룹·그룹 역할 확인](/guide/account.png)

## 활성 그룹 전환하기

여러 그룹에 속해 있다면 상단 막대의 **그룹 선택기**로 현재 컨텍스트를 바꿀 수 있습니다. 전환하면 이후 만드는 세션·볼륨의 소속과 과금 맥락이 선택한 그룹으로 바뀝니다.

![상단 그룹 선택기 — 활성 그룹(컨텍스트) 전환](/guide/group-switch.png)

## 한눈에 비교

| 구분 | 조직 | 그룹 |
| --- | --- | --- |
| 범위 | 단과대·센터 등 최상위 | 연구실·프로젝트 |
| 크레딧 | 전체 풀의 출발점 | 실제 예산·지갑 적용 |
| 관리자 | 조직 관리자 | 그룹 관리자 |
| 내 활동 | — | 세션·볼륨 생성 단위 |

그룹에 아직 속해 있지 않다면 \`그룹 가입 신청\`에서 가입을 요청할 수 있습니다(\`그룹 가입하기\` 가이드 참고).`,
      en: `# Organizations & Groups

Membership in Giosk has two layers: **Organization** and **Group**. Telling them apart explains where credits come from and how resource policy applies.

## Organization

An organization is the **top-level unit**, such as a college or research center. It is the source of the overall credit pool and policy, and it contains several groups. For example, the \`AI응용연구센터\` organization contains groups like \`비전 연구실\` and \`자연어처리 연구실\`.

## Group

A group is the **lab or project unit** where budget and resources actually apply. Session cost is managed with the group wallet, and a group admin runs members and budget. Your role within a group is one of:

- **Member**: creates sessions and volumes and uses resources.
- **Project admin**: manages members, budget, and join requests.

Check your membership and group role on the **Membership** card in \`My settings\`.

![My settings — organization, group, and group role](/guide/account.png)

## Switching the active group

If you belong to several groups, use the **group selector** in the top bar to switch context. After switching, sessions and volumes you create are attached to and billed in the chosen group.

![Top-bar group selector — switch the active group (context)](/guide/group-switch.png)

## Side-by-side

| Aspect | Organization | Group |
| --- | --- | --- |
| Scope | Top-level (college/center) | Lab / project |
| Credits | Source of the overall pool | Where budget/wallet apply |
| Admin | Org admin | Group (project) admin |
| Your activity | — | Unit for sessions/volumes |

If you do not belong to a group yet, request to join under \`Join a group\` (see the \`Joining a Group\` guide).`,
    },
  },
  {
    id: 'join',
    title: { ko: '그룹 가입하기', en: 'Joining a Group' },
    body: {
      ko: `# 그룹 가입하기

세션을 만들고 자원을 쓰려면 그룹에 속해 있는 것이 좋습니다. 그룹에 가입하면 그룹의 예산·공유 볼륨·정책을 함께 사용할 수 있습니다.

## 가입 가능한 그룹 찾기

\`그룹 가입 신청\` 화면을 열면 **가입 가능한 그룹** 목록이 나옵니다. 그룹 이름이나 조직으로 검색할 수 있고, 각 그룹에는 소속 조직과 \`내 조직\` 여부가 표시됩니다.

- 이미 속한 그룹은 \`가입됨\`으로 표시됩니다.
- 가입을 받는 그룹에는 \`가입 신청\` 버튼이, 신청을 닫은 그룹에는 \`마감\`이 표시됩니다.

![그룹 가입 신청 — 가입 가능한 그룹 목록과 신청 버튼](/guide/join.png)

## 가입 신청과 상태 확인

원하는 그룹의 \`가입 신청\`을 누르면 신청이 접수되고, 해당 그룹의 관리자가 검토합니다. 신청 내역은 같은 화면 아래 **내 가입 신청 현황**에서 \`대기 → 승인/거절\` 상태로 추적됩니다. 대기 중인 신청은 직접 취소할 수도 있습니다.

승인되면 그 그룹이 내 소속에 추가되고, 상단 **그룹 선택기**에서 해당 그룹으로 전환해 세션을 만들 수 있습니다(\`조직과 그룹의 차이\` 가이드 참고).

> 가입 신청 버튼이 보이지 않거나 \`마감\`으로 표시되면, 그 그룹이 가입을 닫아 둔 것입니다. 그룹 관리자나 운영자에게 직접 추가를 요청하세요.`,
      en: `# Joining a Group

To create sessions and use resources, it helps to belong to a group. Joining a group lets you share its budget, shared volumes, and policy.

## Find a group to join

Open the \`Join a group\` screen to see the **groups you can join**. Search by group name or organization; each group shows its organization and whether it is in \`My org\`.

- Groups you already belong to are marked \`Joined\`.
- Groups that accept joins show a \`Request to join\` button; groups that have closed joins show \`Closed\`.

![Join a group — list of joinable groups with a request button](/guide/join.png)

## Request and track status

Click \`Request to join\` on the group you want; the request is submitted and that group's admin reviews it. Your requests are tracked under **My requests** at the bottom of the screen, moving from \`pending → approved/rejected\`. You can cancel a pending request yourself.

Once approved, the group is added to your membership, and you can switch to it via the top-bar **group selector** to create sessions (see the \`Organizations & Groups\` guide).

> If you do not see a request button or it shows \`Closed\`, that group has closed joins. Ask the group admin or an operator to add you directly.`,
    },
  },
  {
    id: 'alerts',
    title: { ko: '알림 센터', en: 'Alerts' },
    body: {
      ko: `# 알림 센터

알림 센터에서는 원하는 조건을 직접 정의해 이메일이나 웹훅으로 알림을 받을 수 있습니다. 크레딧이 떨어지거나 세션이 유휴 상태로 방치되는 상황을 미리 잡아내는 데 유용합니다.

## 알림 규칙 만들기

\`알림 센터 → 규칙 추가\`를 누르면 새 규칙 행이 생깁니다. **조건 → 비교(이상/이하) → 임계값 → 채널**을 순서대로 고르면 됩니다. 각 규칙은 토글로 켜고 끌 수 있습니다.

![알림 센터 — 규칙 목록과 등록된 이메일·웹훅](/guide/notify.png)

선택할 수 있는 대표 조건(주제)은 다음과 같습니다.

- **크레딧 잔액**: 잔액이 임계값 이하로 떨어지면 알림 (예: \`크레딧 잔액 ≤ 50 C\`).
- **예산 사용률**: 그룹 예산 사용률이 임계값 이상이면 알림 (예: \`예산 사용률 ≥ 80%\`).
- **세션 유휴 시간**: 세션이 일정 시간 이상 유휴면 알림 (예: \`유휴 ≥ 30분\`).
- **대기열 길이**: 자원 대기열이 길어지면 알림.
- **GPU 사용률** 등 워크로드 지표 기반 조건.

## 알림 채널 등록

화면 아래 **등록된 이메일**과 **등록된 웹훅**에 받을 주소를 추가하세요. 규칙의 채널을 \`이메일\` 또는 \`웹훅\`으로 지정하면, 조건이 충족될 때 해당 주소로 발송됩니다. 웹훅은 Slack 등 외부 도구로 연동할 때 유용합니다.

## 가용성 알림

\`워크로드 알림\` 영역에서는 지금 가용하지 않은 GPU 모델·노드에 대해 **사용 가능해지면 알려 주는** 알림을 등록할 수 있습니다. 세션 마법사에서 \`가용 없음\`인 자원을 만났을 때 \`알림 신청\`을 누르면 같은 알림이 등록됩니다.

> 알림은 받기만 하는 안내이며, 세션을 자동으로 만들지는 않습니다. 가용 알림을 받으면 직접 세션을 생성하세요.`,
      en: `# Alerts

The alert center lets you define your own conditions and get notified by email or webhook. It is useful for catching low credits or sessions left idle before they cost you.

## Create an alert rule

Click \`Alerts → Add rule\` to add a new rule row. Choose **condition → comparison (≥/≤) → threshold → channel** in order. Each rule can be toggled on or off.

![Alerts — rule list and registered emails/webhooks](/guide/notify.png)

Common conditions (topics) include:

- **Credit balance**: notify when the balance falls below a threshold (e.g. \`balance ≤ 50 C\`).
- **Budget usage**: notify when group budget usage exceeds a threshold (e.g. \`usage ≥ 80%\`).
- **Session idle time**: notify when a session is idle beyond a duration (e.g. \`idle ≥ 30 min\`).
- **Queue length**: notify when the resource queue grows long.
- Workload-metric conditions such as **GPU utilization**.

## Register notification channels

Add the addresses to notify under **Registered emails** and **Registered webhooks** at the bottom. Set a rule's channel to \`email\` or \`webhook\`, and it is sent there when the condition is met. Webhooks are handy for integrating with tools like Slack.

## Availability alerts

In the \`Workload alerts\` area you can register an alert that **tells you when a currently unavailable GPU model or node becomes available**. Clicking \`Request alert\` on an unavailable resource in the session wizard registers the same alert.

> Alerts are notifications only — they do not create a session for you. When you get an availability alert, create the session yourself.`,
    },
  },
];
