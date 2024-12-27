package pro

import (
	"context"
	"time"
)

type AuthAPI interface {
	// public
	Login(ctx context.Context, req LoginRequest) (reply LoginReply, err error)
	SendVerificationCode(ctx context.Context, req VerificationCodeRequest) (reply VerificationCodeReply, err error)
	MailExists(ctx context.Context, req MailExistsRequest) (reply MailExistsReply, err error)
	ResetPasswordByCode(ctx context.Context, req ResetPasswordByCodeRequest) (err error)
	ValidInvite(ctx context.Context, req InviteCodeJudgeReq) (InviteCodeJudgeReply, error)
	// private
	UserInfo(ctx context.Context, req Empty) (info UserInfo, err error)
	UpdateUserInfo(ctx context.Context, req UpdateUserInfoRequest) (err error)
	UserInviteCode(ctx context.Context, req Empty) (UserInviteCodeReply, error)
	UserInviteRecord(ctx context.Context, req Empty) (UserInviteRecordReply, error)
}

type Empty struct {
}
type UserInviteCodeReply struct {
	InviteCode string `json:"invite_code"`
}
type UserInviteRecord struct {
	UserEmail    string    `json:"user_email"`
	RegisterTime time.Time `json:"register_time"`
	IsValid      bool      `json:"is_valid"`
}

type UserInviteRecordReply struct {
	Items []UserInviteRecord `json:"items"`
}

type UserInfo struct {
	Name        string         `json:"name"`
	Mail        string         `json:"mail"`
	LastLoginAt int64          `json:"last_login_at"`
	Mtype       MemberShipType `json:"membership_type"`
	ExpiredTime int64          `json:"expired_time"`
}

type UpdateUserInfoRequest struct {
	Name        string `json:"name"`
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type LoginRequest struct {
	Mail             string `json:"mail"`
	Password         string `json:"password"`
	VerificationCode string `json:"verification_code"`
	Code             string `json:"code"`
	Token            string `json:"token"`
	InviteCode       string `json:"invite_code"`
}

type InviteCodeJudgeReq struct {
	Mail       string `json:"mail"`
	InviteCode string `json:"invite_code"`
}

type InviteCodeJudgeReply struct {
	Success bool   `json:"success"`
	Msg     string `json:"msg"`
}

type LoginReply struct {
	Id         uint    `json:"id"`
	Token      string  `json:"token"`
	ExpiredAt  int64   `json:"expired_at"`
	Lang       string  `json:"lang"`
	Name       *string `json:"name"`
	Mail       string  `json:"mail"`
	IsActivity bool    `json:"is_activity"`
}

type VerificationCodeRequest struct {
	Mail string `json:"mail"`
}

type VerificationCodeReply struct {
	Token string `json:"token"`
}

type RegisterRequest struct {
	Mail       string `json:"mail"`
	Password   string `json:"password"`
	Code       string `json:"code"`
	Token      string `json:"token"`
	InviteCode string `json:"invite_code"`
}

type ResetPasswordCodeRequest struct {
	Mail string `json:"mail"`
}

type ResetPasswordByCodeRequest struct {
	Mail        string `json:"mail"`
	Code        string `json:"code"`
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type ResetPasswordCodeReply struct {
	Token string `json:"token"`
}

type MailExistsRequest struct {
	Mail string `json:"mail"`
}

type MailExistsReply struct {
	Exists bool `json:"exists"`
}
