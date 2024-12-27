package auth

import (
	"context"
	"fmt"
	rand1 "math/rand"
	"os"
	"strings"
	"time"

	"github.com/golang-module/carbon/v2"
	redigo "github.com/gomodule/redigo/redis"
	"github.com/gozelle/dongle"
	"github.com/gozelle/mail"
	"github.com/gozelle/mix"
	"github.com/gozelle/rand"
	"github.com/gozelle/uuid"
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/bearer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/biz/proutils"
	prodal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/dal"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/vip"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

var InviteStartTime time.Time

func init() {
	InviteStartTime = time.Date(2023, 11, 20, 0, 0, 0, 0, chain.TimeLoc)
	if s := os.Getenv("INVITE_START_TIME"); s != "" {
		st, err := time.Parse(carbon.DateTimeLayout, s)
		if err != nil {
			panic(err)
		}

		InviteStartTime = st
	}
}

func NewAuth(conf *config.Pro, db *gorm.DB, m *mail.Client, r *redis.Redis) *AuthBiz {
	return &AuthBiz{
		mail:       m,
		repo:       prodal.NewAuthDal(db),
		conf:       conf,
		redis:      r,
		db:         db,
		inviteRepo: dal.NewInviteDal(db),
	}
}

var _ pro.AuthAPI = (*AuthBiz)(nil)

type AuthBiz struct {
	mail       *mail.Client
	repo       prorepo.AuthRepo
	conf       *config.Pro
	redis      *redis.Redis
	db         *gorm.DB
	inviteRepo repository.InviteCodeRepo
}

func (a AuthBiz) UserInviteCode(ctx context.Context, req pro.Empty) (pro.UserInviteCodeReply, error) {
	b := bearer.UseBearer(ctx)
	code, err := a.inviteRepo.GetUserInviteCode(ctx, int(b.Id))
	if err == nil {
		return pro.UserInviteCodeReply{InviteCode: code.Code}, nil
	}
	res := ""
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		logger.Errorf("get user invite code failed :%w", err)
		return pro.UserInviteCodeReply{}, err
	} else {
		for {
			codes := generateInviteCode(6)
			err = a.inviteRepo.SaveUserInviteCode(ctx, int(b.Id), codes)
			if err == nil {
				res = codes
				break
			} else {
				logger.Errorf("save invite code failed: %w, req.id %d, codes %s", err, b.Id, codes)
				time.Sleep(time.Second)
			}
		}
	}
	return pro.UserInviteCodeReply{InviteCode: res}, nil
}

func (a AuthBiz) UserInviteRecord(ctx context.Context, req pro.Empty) (pro.UserInviteRecordReply, error) {
	b := bearer.UseBearer(ctx)
	code, err := a.inviteRepo.GetUserInviteCode(ctx, int(b.Id))
	if err != nil {
		return pro.UserInviteRecordReply{}, err
	}
	records, err := a.inviteRepo.GetUserInviteRecordByCode(ctx, code.Code)
	if err != nil {
		return pro.UserInviteRecordReply{}, err
	}
	reply := pro.UserInviteRecordReply{}
	for i := range records {
		reply.Items = append(reply.Items, pro.UserInviteRecord{
			UserEmail:    records[i].UserEmail,
			RegisterTime: records[i].RegisterTime,
			IsValid:      records[i].IsValid && (records[i].RegisterTime.After(InviteStartTime)),
		})
	}
	return reply, nil
}

func (a AuthBiz) UpdateUserInfo(ctx context.Context, req pro.UpdateUserInfoRequest) (err error) {

	b := bearer.UseBearer(ctx)

	tx := a.db.Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	ctx = _dal.ContextWithDB(ctx, tx)

	if req.Name != "" {
		err = a.repo.UpdateUserName(ctx, b.Id, req.Name)
		if err != nil {
			return
		}
	}

	if req.NewPassword == "" {
		return
	}

	u, err := a.repo.GetUserById(ctx, b.Id)
	if err != nil {
		return
	}

	if !ComparePasswords(req.OldPassword, u.Password) {
		err = mix.Warnf("invliad origin password")
		return
	}

	err = a.repo.UpdateUserPassword(ctx, u.Id, EncodePassword(req.NewPassword))
	if err != nil {
		return
	}

	return
}

func (a AuthBiz) UserInfo(ctx context.Context, req pro.Empty) (info pro.UserInfo, err error) {
	b := bearer.UseBearer(ctx)
	v := vip.UseVIP(ctx)
	user, err := a.repo.GetUserById(ctx, b.Id)
	if err != nil {
		return
	}

	if user.Name != nil {
		info.Name = *user.Name
	} else {
		info.Name = user.Mail
	}
	info.Mail = user.Mail
	info.LastLoginAt = user.LastLoginAt.Unix()
	info.Mtype = v.MType
	info.ExpiredTime = v.ExpiredTime.Unix()
	return
}

func (a AuthBiz) MailExists(ctx context.Context, req pro.MailExistsRequest) (reply pro.MailExistsReply, err error) {
	user, err := a.repo.GetUserByMailOrNil(ctx, req.Mail)
	if err != nil {
		return
	}
	reply.Exists = user != nil
	return
}

func (a AuthBiz) ValidInvite(ctx context.Context, req pro.InviteCodeJudgeReq) (pro.InviteCodeJudgeReply, error) {
	user, err := a.repo.GetUserByMailOrNil(ctx, req.Mail)
	if err != nil {
		logger.Errorf("get user by email failed %w", err)
		return pro.InviteCodeJudgeReply{}, err
	}

	if user == nil {
		return pro.InviteCodeJudgeReply{
			Success: true,
			Msg:     "",
		}, nil
	}

	// TODO: correct time
	if user.CreatedAt.After(InviteStartTime) {
		item, err := a.inviteRepo.GetUserInviteRecordByUserID(ctx, int(user.Id))
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return pro.InviteCodeJudgeReply{}, err
		} else if err != nil {
			return pro.InviteCodeJudgeReply{Success: true, Msg: ""}, nil //nolint
		} else {
			if req.InviteCode != item.Code {
				return pro.InviteCodeJudgeReply{
					Success: false,
					Msg:     "invite code mismatch",
				}, nil
			}
		}
	}
	return pro.InviteCodeJudgeReply{
		Success: true,
		Msg:     "",
	}, nil
}

// func (a AuthBiz) Login(ctx context.Context, req pro.LoginRequest) (pro.LoginReply, error) {
func (a AuthBiz) Login(ctx context.Context, req pro.LoginRequest) (reply pro.LoginReply, err error) {
	req.Code = strings.TrimSpace(req.Code)
	tx := a.db.Begin()
	ctx = _dal.ContextWithDB(ctx, tx)
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()
	if req.Code != "" {
		return a.registerByCode(ctx, pro.RegisterRequest{
			Mail:       req.Mail,
			Password:   req.Password,
			Code:       req.Code,
			Token:      req.Token,
			InviteCode: req.InviteCode,
		})
	}

	user, err := a.repo.GetUserByMailOrNil(ctx, req.Mail)
	if err != nil {
		logger.Errorf("get user by email failed %w", err)
		return pro.LoginReply{}, err
	}
	if user == nil {
		err = mix.Warnf("invalid mail or password")
		logger.Warnf("user mismatch of %s", req.Mail)
		return pro.LoginReply{}, err
	}

	if !ComparePasswords(req.Password, user.Password) {
		err = mix.Warnf("invalid mail or password")
		logger.Warnf("password mismatch of %s", req.Mail)
		return pro.LoginReply{}, err
	}

	_ = a.repo.UpdateUserLoginTime(ctx, user.Id, time.Now(), user.LoginAt)
	// 判断是否参与过活动
	isActivity, err := a.participateActivitiesOrNot(ctx, user.Id)
	if err != nil {
		logger.Errorf("participate activities failed %w", err)
		return pro.LoginReply{}, err
	}
	return a.genLoginToken(user, isActivity)
}

// 包装一个活动接口，未来新的活动可以只调整里面内容
func (a AuthBiz) participateActivitiesOrNot(ctx context.Context, userID int64) (isActivity bool, err error) {
	// 从用户表中判断有没有参加过活动（未来：新的活动可以数据库中统一将活动标志置空，可开启新一轮活动）
	isActivity, err = a.repo.GetActivityStateAndSetTrue(ctx, userID)
	if err != nil {
		return
	}
	if !isActivity {
		// 当前活动放在这里，目前是第一次送会员
		_, err = proutils.InternalRechargeMembership(ctx, a.db, a.redis, userID, pro.EnterpriseVIP, "7d")
		if err != nil {
			logger.Errorf("complimentary membership failed: %w", err)
			return
		}
	}
	// false表示以前没参与过活动，第一次参加活动，true表示曾经参与过活动
	return isActivity, nil
}

func (a AuthBiz) genLoginToken(user *propo.User, isActivity bool) (reply pro.LoginReply, err error) {
	token, expiredAt, err := GenJWT(a.conf.JwtSecret, &bearer.Bearer{
		Id:   user.Id,
		Mail: user.Mail,
	})
	if err != nil {
		return
	}

	reply.Id = uint(user.Id)
	reply.Token = token
	reply.ExpiredAt = expiredAt.Unix()
	reply.Lang = "zh-CN"
	reply.Name = user.Name
	reply.Mail = user.Mail
	reply.IsActivity = isActivity
	return
}

func (a AuthBiz) makeCodeToken(mail string) string {
	return dongle.Encrypt.FromString(fmt.Sprintf("%s%d", mail, time.Now().Nanosecond())).BySha1().ToHexString()
}

func (a AuthBiz) SendVerificationCode(ctx context.Context, req pro.VerificationCodeRequest) (reply pro.VerificationCodeReply, err error) {

	req.Mail = strings.TrimSpace(req.Mail)

	if req.Mail == "" {
		err = fmt.Errorf("invalid mail")
		return
	}

	m := mail.NewMsg()
	err = m.From("FilscanTeam<admin@filscan.io>")
	if err != nil {
		return
	}

	err = m.To(req.Mail)
	if err != nil {
		return
	}

	code := rand.Code(6)
	token := a.makeCodeToken(req.Mail)

	err = a.redis.Set(token, code, 10*time.Minute)
	if err != nil {
		return
	}

	logger.Infof("set token %s code %s", token, code)

	m.Subject("Register code")
	m.SetBodyString(mail.TypeTextHTML, strings.ReplaceAll(codeTpl, "$code", code))

	err = a.mail.DialAndSend(m)
	if err != nil {
		return
	}

	reply.Token = token
	return
}

// todo 该命名存在一定迷惑性，该函数也可以用于验证码登录和注册
func (a AuthBiz) registerByCode(ctx context.Context, req pro.RegisterRequest) (reply pro.LoginReply, err error) {
	if req.Code == "" || req.Token == "" {
		err = mix.Warnf("invalid code")
		logger.Errorf("invalid code: req.code == nil or req.token == nil")
		return pro.LoginReply{}, err
	}

	r, err := a.redis.Get(req.Token)
	if err != nil {
		if errors.Is(err, redigo.ErrNil) {
			err = mix.Warnf("invalid code")
			logger.Errorf("can't get code from redis")
		}
		return pro.LoginReply{}, err
	}
	logger.Infof("set token %s code %s", req.Token, string(r))
	if fmt.Sprintf("\"%s\"", req.Code) != string(r) {
		err = mix.Warnf("invalid code")
		logger.Errorf("req.code != string r")
		return pro.LoginReply{}, err
	}

	user, err := a.repo.GetUserByMailOrNil(ctx, req.Mail)
	if err != nil {
		logger.Errorf("get user by mail failed : %w", err)
		return pro.LoginReply{}, err
	}
	newFlag := false
	if user == nil {
		newFlag = true
		if strings.TrimSpace(req.Password) == "" { //注册部分逻辑，保存密码
			u, _ := uuid.NewUUID()
			req.Password = u.String()
		}
		user = &propo.User{
			Id:          0,
			Name:        nil,
			Mail:        req.Mail,
			Password:    EncodePassword(req.Password),
			LoginAt:     time.Now(),
			LastLoginAt: time.Now(),
		}
		err = a.repo.SaveUser(ctx, user)
		if err != nil {
			logger.Errorf("save user failed: %w", err)
			return pro.LoginReply{}, err
		}

		inviteCode := generateInviteCode(6)
		err = a.inviteRepo.SaveUserInviteCode(ctx, int(user.Id), inviteCode)
		if err != nil {
			logger.Errorf("save invite code failed: %w", err)
		}
	}

	if req.InviteCode != "" && newFlag {
		err = a.inviteRepo.SaveUserInviteRecord(ctx, int(user.Id), req.InviteCode, req.Mail, user.CreatedAt)
		if err != nil {
			logger.Errorf("save invite record failed %s %s %w", user.Id, req.InviteCode, err)
			return pro.LoginReply{}, fmt.Errorf("invalid invite code")
		}
	}
	// 所以这里不一定只是注册，也要进行是否参加过活动的判断
	isActivity, err := a.participateActivitiesOrNot(ctx, user.Id)
	if err != nil {
		logger.Errorf("participate activities failed %w", err)
		return pro.LoginReply{}, err
	}
	_, _ = a.redis.Delete(req.Token)
	return a.genLoginToken(user, isActivity)
}
func generateInviteCode(length int) string {
	// 定义邀请码字符集
	charset := "abcdefghijklmnopqrstuvwxyz"
	code := make([]byte, length)

	// 使用当前时间作为随机数种子
	rand1.Seed(time.Now().UnixNano())

	// 生成随机邀请码
	for i := 0; i < length; i++ {
		index := rand.Intn(len(charset))
		code[i] = charset[index]
	}

	return string(code)
}

//func (a AuthBiz) SendResetPasswordCode(ctx context.Context, req pro.ResetPasswordCodeRequest) (reply pro.ResetPasswordCodeReply, err error) {
//
//	req.Mail = strings.TrimSpace(req.Mail)
//
//	if req.Mail == "" {
//		err = fmt.Errorf("invalid mail")
//		return
//	}
//
//	m := mail.NewMsg()
//	err = m.From("admin@filscan.io")
//	if err != nil {
//		return
//	}
//
//	err = m.To(req.Mail)
//	if err != nil {
//		return
//	}
//
//	code := rand.Code(6)
//	token := a.makeCodeToken(req.Mail)
//
//	err = a.redis.Set(token, code, 10*time.Minute)
//	if err != nil {
//		return
//	}
//
//	m.Subject("Reset password code")
//	m.SetBodyString(mail.TypeTextHTML, strings.ReplaceAll(codeTpl, "$code", code))
//
//	err = a.mail.DialAndSend(m)
//	if err != nil {
//		return
//	}
//
//	reply.Token = token
//
//	return
//}

func (a AuthBiz) ResetPasswordByCode(ctx context.Context, req pro.ResetPasswordByCodeRequest) (err error) {

	if req.Code == "" || req.Token == "" {
		err = mix.Warnf("invalid code")
		return
	}

	r, err := a.redis.Get(req.Token)
	if err != nil {
		if errors.Is(err, redigo.ErrNil) {
			err = mix.Warnf("invalid code")
		}
		return
	}

	if fmt.Sprintf("\"%s\"", req.Code) != string(r) {
		err = mix.Warnf("invalid code")
		return
	}

	user, err := a.repo.GetUserByMailOrNil(ctx, req.Mail)
	if err != nil {
		return
	}
	if user == nil {
		err = mix.Warnf("invalid mail")
		return
	}

	err = a.repo.UpdateUserPassword(ctx, user.Id, EncodePassword(req.NewPassword))
	if err != nil {
		return
	}

	_, _ = a.redis.Delete(req.Token)

	return
}
