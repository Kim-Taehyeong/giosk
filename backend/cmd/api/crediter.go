package main

import (
	"fmt"

	"giosk/internal/org"
	"giosk/internal/topup"
	"giosk/internal/wallet"
)

// crediter는 topup.Crediter 구현 — 승인 시 target 종류별로 입금 라우팅.
//
//	user → 개인 지갑, group → 그룹 지갑, org → 조직 풀.
type crediter struct {
	wallet  *wallet.Service
	orgRepo org.Repository
}

func (c crediter) Credit(targetType string, targetID int64, amount int, actor int64) error {
	switch targetType {
	case topup.TargetUser:
		return c.wallet.CreditUser(targetID, 0, amount, "topup approved", &actor)
	case topup.TargetGroup:
		return c.wallet.CreditGroup(targetID, amount, &actor)
	case topup.TargetOrg:
		return c.orgRepo.AddCredit(targetID, int64(amount))
	default:
		return fmt.Errorf("unknown topup target: %s", targetType)
	}
}
