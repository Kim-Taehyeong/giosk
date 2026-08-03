# 설계 결정 기록 (ADR)

되돌리기 어렵거나, 나중에 "왜 이렇게 했지"를 반드시 다시 묻게 될 결정만 기록한다.
각 문서는 **맥락 / 결정 / 이유 / 대안 / 결과**로 쓴다. 결정이 뒤집히면 문서를 지우지 말고
상태를 `대체됨`으로 바꾸고 새 번호를 만든다.

| 번호 | 제목 | 상태 |
|------|------|------|
| [0001](0001-kubernetes-native-sessions.md) | 세션을 네이티브 Pod로 실행한다 | 확정 |
| [0002](0002-mysql-system-of-record.md) | 도메인 상태는 MySQL에 둔다 | 확정 |
| [0003](0003-exclusive-vs-shared-gpu.md) | 전용 GPU와 공유(HAMi) GPU를 분리 집계한다 | 확정 |
| [0004](0004-session-local-home.md) | 세션 홈은 노드 로컬 영속 볼륨으로 둔다 | 확정 |
| [0005](0005-idle-detection.md) | 유휴 판정에 전력을 보조 신호로 쓴다 | 확정 |
| [0006](0006-vm-leasing-deferred.md) | VM 임대(KubeVirt + PCI 패스스루)는 보류 | 보류 |
