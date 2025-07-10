package pro

import (
	"context"
	"time"
)

type MemberShipType string

const (
	EnterpriseVIP    MemberShipType = "EnterpriseVIP"
	EnterpriseProVIP MemberShipType = "EnterpriseProVIP"
	NormalVIP        MemberShipType = "NormalVIP"
)

type MemberShipAPI interface {
	// 目前是手工开通，因此给与了什么类型，就给开通多长时间（自动更改类型）
	RechargeMembership(ctx context.Context, req RechargeMembershipRequest) (resp *RechargeMembershipResponse, err error)
}

type RechargeMembershipRequest struct {
	HashKey    string         `json:"hash_key,omitempty"`
	UserID     int64          `json:"user_id,omitempty"`
	MType      MemberShipType `json:"membership_type,omitempty"`
	ExtendTime string         `json:"extend_time,omitempty"`
}

type RechargeMembershipResponse struct {
	MType       MemberShipType `json:"membership_type"`
	ExpiredTime time.Time      `json:"expired_time"`
}
