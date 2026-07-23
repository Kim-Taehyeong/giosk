package topup

import "fmt"

// Crediter는 승인 시 대상(user/group/org)에 크레딧을 입금한다(main 에서 wallet/org 로 라우팅).
type Crediter interface {
	Credit(targetType string, targetID int64, amount int, actor int64) error
}

// Service는 topup 비즈니스 로직.
type Service struct {
	repo     Repository
	crediter Crediter
}

func NewService(repo Repository, crediter Crediter) *Service {
	return &Service{repo: repo, crediter: crediter}
}

func (s *Service) Create(requesterID int64, req CreateReq) error {
	targetID := req.TargetID
	if req.TargetType == TargetUser && targetID == 0 {
		targetID = requesterID // 개인 충전 요청은 본인 대상
	}
	return s.repo.Create(&Request{
		RequesterUserID: requesterID,
		TargetType:      req.TargetType,
		TargetID:        targetID,
		Amount:          req.Amount,
		Reason:          req.Reason,
		Status:          StatusPending,
	})
}

func (s *Service) MyRequests(userID int64) ([]Item, error)        { return s.repo.MyRequests(userID) }
func (s *Service) PendingForGroup(groupID int64) ([]Item, error)  { return s.repo.PendingForGroup(groupID) }
func (s *Service) PendingForOrg(orgID int64) ([]Item, error)      { return s.repo.PendingForOrg(orgID) }
func (s *Service) PendingAll() ([]Item, error)                    { return s.repo.PendingAll() }

// Approve는 대상에 크레딧을 입금하고 요청을 승인 처리한다(입금 성공 후 마크).
func (s *Service) Approve(id, reviewer int64) error {
	req, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	if req.Status != StatusPending {
		return fmt.Errorf("already reviewed")
	}
	if err := s.crediter.Credit(req.TargetType, req.TargetID, req.Amount, reviewer); err != nil {
		return err
	}
	return s.repo.MarkReviewed(id, reviewer, StatusApproved)
}

func (s *Service) Reject(id, reviewer int64) error {
	return s.repo.MarkReviewed(id, reviewer, StatusRejected)
}
