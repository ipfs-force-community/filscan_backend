package probiz

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"

	"github.com/gozelle/async/collection"
	"github.com/gozelle/mix"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	mbiz "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/service"
	mdal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/dal"
	mrepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/repo"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/bearer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/biz/auth"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/biz/proutils"
	prodal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/dal"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/vip"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

const defaultGroupName = "default_group"

func NewGroup(db *gorm.DB, adapter londobell.Adapter, redisClient *redis.Redis) *GroupBiz {
	return &GroupBiz{
		groupRepo:        prodal.NewGroupDal(db),
		userMinerRepo:    prodal.NewUserMinerDal(db),
		ruleRepo:         mdal.NewRuleDal(db),
		db:               db,
		adapter:          adapter,
		redisClient:      redisClient,
		inviteRecordRepo: dal.NewInviteDal(db),
	}
}

var _ pro.GroupAPI = (*GroupBiz)(nil)

type GroupBiz struct {
	groupRepo        prorepo.GroupRepo
	userMinerRepo    prorepo.UserMinerRepo
	ruleRepo         mrepo.RuleRepo
	db               *gorm.DB
	adapter          londobell.Adapter
	mutex            sync.Mutex
	redisClient      *redis.Redis
	inviteRecordRepo repository.InviteCodeRepo
}

//func (g *GroupBiz) checkUserGroups(ctx context.Context, groupId int64) (result bool, err error) {
//	b := bearer.UseBearer(ctx)
//	groups, err := g.groupRepo.SelectGroupsByUserID(ctx, b.Id)
//	if err != nil {
//		return
//	}
//	if groups == nil {
//		return
//	}
//	for _, group := range groups {
//		if groupId == group.Id {
//			result = true
//			return
//		}
//	}
//	err = mix.Codef(201, "Current user did not have group: %d !", groupId)
//	return
//}

//func (g *GroupBiz) checkUserMiners(ctx context.Context, minerId chain.SmartAddress) (result bool, err error) {
//	b := bearer.UseBearer(ctx)
//	miners, err := g.userMinerRepo.SelectMinersByUserID(ctx, b.Id)
//	if err != nil {
//		return
//	}
//	for _, userMiner := range miners {
//		if minerId == userMiner.MinerID {
//			result = true
//			return
//		}
//	}
//	err = mix.Codef(202, "Current user did not have miner: %s !", minerId.Address())
//	return
//}

//func (g *GroupBiz) checkMinerCountReduplicate(ctx context.Context, minerList []*pro.MinerInfo, groupID *int64) error {
//	b := bearer.UseBearer(ctx)
//	userMiners, err := g.userMinerRepo.SelectMinersByUserID(ctx, b.Id)
//	if err != nil {
//		return err
//	}
//
//	for _, userMiner := range userMiners {
//		if groupID != nil && userMiner.GroupID == *groupID {
//			continue
//		}
//		minerList = append(minerList, &pro.MinerInfo{
//			MinerID:  userMiner.MinerID,
//			MinerTag: userMiner.MinerTag,
//		})
//	}
//
//	totalCount := len(minerList)
//	if totalCount > 10 {
//		return mix.Codef(203, "The total miner count of current user over than 10!")
//	}
//	visited := make(map[chain.SmartAddress]bool)
//	for _, minerInfo := range minerList {
//		if visited[minerInfo.MinerID] {
//			return mix.Codef(205, "Miner: %s is reduplicated!", minerInfo.MinerID)
//		}
//		visited[minerInfo.MinerID] = true
//	}
//
//	return nil
//}

func (g *GroupBiz) checkValidMiners(ctx context.Context, origin []*propo.UserMiner, minerList []*pro.MinerInfo) error {

	m := map[string]struct{}{}
	for _, v := range origin {
		m[v.MinerID.Address()] = struct{}{}
	}

	for _, minerInfo := range minerList {
		minerInfo.MinerID = chain.SmartAddress(strings.TrimSpace(string(minerInfo.MinerID)))
		if _, ok := m[minerInfo.MinerID.Address()]; ok {
			continue
		}
		if !minerInfo.MinerID.IsID() {
			return mix.Codef(204, "Miner: %s not found!", minerInfo.MinerID.Address())
		}
		actor, err := g.adapter.Actor(ctx, minerInfo.MinerID, nil)
		if err != nil {
			return mix.Codef(204, "Miner: %s not found!", minerInfo.MinerID.Address())
		}
		if actor == nil || actor.ActorType != "miner" {
			return mix.Codef(204, "Miner: %s not found!", minerInfo.MinerID.Address())
		}
	}

	return nil
}

//func (g *GroupBiz) checkGroupNameExist(ctx context.Context, groupName string, groupID *int64) error {
//	b := bearer.UseBearer(ctx)
//	groupInfos, err := g.groupRepo.SelectGroupsByUserID(ctx, b.Id)
//	if err != nil {
//		return err
//	}
//	for _, group := range groupInfos {
//		if group.GroupName == groupName {
//			if groupID != nil && group.Id == *groupID {
//				return nil
//			}
//			return mix.Codef(206, "Group name: %s is exist!", groupName)
//		}
//	}
//	return nil
//}

func (g *GroupBiz) GetUserGroups(ctx context.Context, req pro.GetUserGroupsRequest) (resp pro.GetUserGroupsResponse, err error) {
	b := bearer.UseBearer(ctx)
	v := vip.UseVIP(ctx)
	maxMinersCount := VipMaxMinersCount(v.MType)
	cnt := int64(0) //统计，不能超过最大值
	defaultGroup := &pro.GroupInfo{
		GroupID:    0,
		GroupName:  defaultGroupName,
		IsDefault:  true,
		MinersInfo: nil,
	}

	// 准备 Groups
	groupInfos, err := g.groupRepo.SelectGroupsByUserID(ctx, b.Id)
	if err != nil {
		return
	}

	groupMinerMap := map[int64]*pro.GroupInfo{
		0: defaultGroup,
	}
	for _, v := range groupInfos {
		groupMinerMap[v.Id] = &pro.GroupInfo{
			GroupID:    v.Id,
			GroupName:  v.GroupName,
			IsDefault:  false,
			MinersInfo: nil,
		}
	}

	userMiners, err := g.userMinerRepo.SelectMinersByUserID(ctx, b.Id)
	if err != nil {
		return
	}
	for _, miner := range userMiners {
		groupID := int64(0) // user_miners数据库中，group_id为null时候，表示为默认的分组
		if miner.GroupID != nil {
			groupID = *miner.GroupID
		}
		groupMinerMap[groupID].MinersInfo = append(groupMinerMap[groupID].MinersInfo, &pro.MinerInfo{
			MinerID:  miner.MinerID,
			MinerTag: miner.MinerTag,
		})
		cnt++
		if cnt >= maxMinersCount {
			break
		}
	}

	resp.GroupInfoList = append(resp.GroupInfoList, defaultGroup)
	for _, v := range groupMinerMap {
		if v.GroupID != 0 {
			resp.GroupInfoList = append(resp.GroupInfoList, v)
		}
	}

	collection.Sort[*pro.GroupInfo](resp.GroupInfoList, func(a, b *pro.GroupInfo) bool {
		return a.GroupID < b.GroupID
	})

	return
}

func (g *GroupBiz) GetGroup(ctx context.Context, req pro.GetGroupRequest) (resp pro.GetGroupResponse, err error) {
	b := bearer.UseBearer(ctx)
	groups, err := g.groupRepo.SelectActiveGroupsByUserID(ctx, b.Id)
	if err != nil {
		return
	}
	for _, group := range groups {
		if group.Id > 0 {
			resp.GroupList = append(resp.GroupList, &pro.GroupName{
				GroupID:   group.Id,
				GroupName: group.GroupName,
				IsDefault: false,
			})
		} else {
			resp.GroupList = append([]*pro.GroupName{{
				GroupID:   0,
				GroupName: defaultGroupName,
				IsDefault: true,
			}}, resp.GroupList...)
		}
	}
	return
}

func (g *GroupBiz) DeleteGroup(ctx context.Context, req pro.DeleteGroupRequest) (resp pro.DeleteGroupResponse, err error) {

	b := bearer.UseBearer(ctx)

	tx := g.db.Begin()
	ctx = _dal.ContextWithDB(ctx, tx)
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	err = g.groupRepo.DeleteGroup(ctx, req.GroupID)
	if err != nil {
		return
	}

	err = g.userMinerRepo.DeleteUserMinerByGroupID(ctx, b.Id, req.GroupID)
	if err != nil {
		return
	}
	_, err = g.ruleRepo.DeleteGroupIDRule(ctx, b.Id, req.GroupID)
	if err != nil {
		return
	}
	err = tx.Commit().Error
	if err != nil {
		return
	}

	return
}

func (g *GroupBiz) DeleteGroupMiners(ctx context.Context, req pro.DeleteGroupMinersRequest) (resp pro.DeleteGroupMinersResponse, err error) {
	b := bearer.UseBearer(ctx)
	tx := g.db.Begin()
	ctx = _dal.ContextWithDB(ctx, tx)
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()
	err = g.userMinerRepo.DeleteUserMinerByMinerID(ctx, b.Id, req.MinerID)
	if err != nil {
		return
	}
	_, err = mbiz.GlobalRuleBiz.RuleRepo.DeleteMinerIDRule(ctx, b.Id, req.MinerID)
	if err != nil {
		return resp, err
	}
	resp.MinerID = req.MinerID
	return
}

func (g *GroupBiz) SaveGroupMiners(ctx context.Context, req pro.SaveGroupMinersRequest) (resp pro.SaveGroupMinersResponse, err error) {
	req.GroupName = strings.TrimSpace(req.GroupName)
	b := bearer.UseBearer(ctx)
	v := vip.UseVIP(ctx)

	tx := g.db.Begin()
	ctx = _dal.ContextWithDB(ctx, tx)
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// 提前预处理分组
	if !req.IsDefault && req.GroupName != "" {
		var group *propo.Group

		if req.GroupID > 0 {
			group, err = g.groupRepo.SelectGroupByID(ctx, req.GroupID)
			if err != nil {
				return
			}
		}

		// 如果名字冲突，则会报添加成功，但是仍然显示原来的记录
		var checkGroup *propo.Group
		if group == nil {
			checkGroup, err = g.groupRepo.GetUserGroupByName(ctx, b.Id, req.GroupName)
			if err != nil {
				return
			}
			if checkGroup != nil {
				err = mix.Codef(231, "Group: %s is reduplicated!", req.GroupName)
				if err != nil {
					return
				}
			}
			group = &propo.Group{
				UserId:    b.Id,
				GroupName: req.GroupName,
				IsDefault: false,
			}
			err = g.groupRepo.CreateGroup(ctx, group)
			if err != nil {
				return
			}
			req.GroupID = group.Id
		} else {
			checkGroup, err = g.groupRepo.GetUserGroupByName(ctx, b.Id, req.GroupName)
			if err != nil {
				return
			}
			if checkGroup != nil && checkGroup.Id != req.GroupID {
				err = mix.Codef(231, "Group: %s is reduplicated!", req.GroupName)
				if err != nil {
					return
				}
			}
			group.GroupName = req.GroupName
			_, err = g.groupRepo.UpdateGroup(ctx, group)
			if err != nil {
				return
			}
		}
	}

	// 检查 Miner 是否在链上存在
	originMiners, err := g.userMinerRepo.SelectMinersByUserID(ctx, b.Id)
	if err != nil {
		return
	}
	err = g.checkValidMiners(ctx, originMiners, req.MinerInfos)
	if err != nil {
		return
	}

	// 在分组 Miner 被删除后，如果添加的 Miner 还能在用户的分组中找到，则说明 Miner 冲突了
	var addrs []string

	// 获得原有分组的所有miner信息，检测有无被删除的节点
	preMiners, err := g.userMinerRepo.SelectGroupMinersByGroupID(ctx, b.Id, req.GroupID)
	if err != nil {
		return
	}
	minerInfosMap := make(map[string]struct{})
	for _, v := range req.MinerInfos {
		addr := v.MinerID.Address()
		addrs = append(addrs, addr)
		minerInfosMap[addr] = struct{}{}

	}
	for _, miner := range preMiners {
		if _, ok := minerInfosMap[miner.MinerID.Address()]; !ok {
			//原来的在新的节点中找不到，说明被删除了
			_, err = mbiz.GlobalRuleBiz.RuleRepo.DeleteMinerIDRule(ctx, b.Id, miner.MinerID.Address())
			if err != nil {
				return resp, err
			}
		}
	}

	// 删除原分组的 Miner
	err = g.userMinerRepo.DeleteUserMinerByGroupID(ctx, b.Id, req.GroupID)
	if err != nil {
		return
	}

	exists, err := g.userMinerRepo.QueryExistsMiners(ctx, b.Id, req.GroupID, addrs)
	if err != nil {
		return
	}
	for k := range exists {
		err = mix.Codef(205, "Miner: %s is reduplicated!", k)
		return
	}

	// 保存分组 Miner
	miners, err := g.saveUserGroupNewMiners(ctx, b.Id, req.GroupID, req.MinerInfos)
	if err != nil {
		return
	}

	err = mbiz.SaveGroupPowerRules(ctx, []int64{req.GroupID})
	if err != nil {
		return resp, err
	}

	resp.GroupID = req.GroupID
	resp.MinersInfo = miners

	// 保存完后检查总数是否超过限制总量，超过后则回滚
	maxMiners := VipMaxMinersCount(v.MType)
	count, err := g.userMinerRepo.CountUserMiners(ctx, b.Id)
	if err != nil {
		return
	}
	if count > maxMiners {
		err = mix.Codef(203, "The total miner count of current user over than %d!", maxMiners)
		return
	}

	// TODO: after events, remove this codes
	inviteCode, err := g.inviteRecordRepo.GetUserInviteRecordByUserID(ctx, int(b.Id))
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return
	}
	if inviteCode.Code != "" && inviteCode.IsValid == false {
		err = g.inviteRecordRepo.UpdateUserIsValid(ctx, b.Id)
		if err != nil {
			return
		}

		items, err := g.inviteRecordRepo.GetUserInviteRecordByCode(ctx, inviteCode.Code)
		if err != nil {
			return resp, err
		}
		cnt := 0
		for i := range items {
			if items[i].IsValid == true && items[i].RegisterTime.After(auth.InviteStartTime) {
				cnt++
			}
		}
		if cnt >= 2 {
			uid, err := g.inviteRecordRepo.GetUserIDByInviteCode(ctx, inviteCode.Code)
			if err != nil {
				return resp, err
			}

			success, err := g.inviteRecordRepo.GetInviteSuccessRecord(ctx, int64(uid))
			if err != nil {
				return resp, err
			}

			if !success {
				_, err = proutils.InternalRechargeMembership(ctx, g.db, g.redisClient, int64(uid), pro.EnterpriseVIP, "30d")
				if err != nil {
					return resp, err
				}

				err = g.inviteRecordRepo.SaveSuccessRecords(ctx, int64(uid))
				if err != nil {
					return resp, err
				}
			}
		}

	}
	err = tx.Commit().Error
	if err != nil {
		return
	}

	return
}

// 专属于miner移动的场合
func (g *GroupBiz) saveUserGroupMiners(ctx context.Context, userID int64, groupID int64, minerInfos []*pro.MinerInfo) (result []*pro.MinerInfo, err error) {
	// 判断这里的miners之前的组，与现在的组有没有变化，若有变化，则更新规则库，将miner 相关的group改成新的组【当然，注意该接口只用于标签改变和移动的时候】
	tx := g.db.Begin()
	ctx = _dal.ContextWithDB(ctx, tx)
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	var minerList []chain.SmartAddress
	for _, info := range minerInfos {
		minerList = append(minerList, info.MinerID)
	}
	userMiners, err := g.userMinerRepo.SelectMinersByMiners(ctx, userID, minerList)
	if err != nil {
		return
	}
	for _, miner := range userMiners {
		var minerGroupID = int64(0)
		if miner.GroupID != nil {
			minerGroupID = *miner.GroupID
		}
		if minerGroupID != groupID {
			_, err = mbiz.GlobalRuleBiz.RuleRepo.UpdateUserMinerGroup(ctx, userID, miner.MinerID.Address(), groupID)
			if err != nil {
				return
			}
			break //因为在移动过程中，一次只有一个属于其他组的miner移入
		}
	}
	var newUserMiner []*propo.UserMiner
	mapping := map[string]struct{}{}
	for _, minerInfo := range minerInfos {
		if _, ok := mapping[minerInfo.MinerID.Address()]; ok {
			continue
		}
		mapping[minerInfo.MinerID.Address()] = struct{}{}
		userMiner := &propo.UserMiner{
			UserID:   userID,
			GroupID:  nil,
			MinerID:  minerInfo.MinerID,
			MinerTag: minerInfo.MinerTag,
		}
		if groupID > 0 {
			userMiner.GroupID = &groupID
		}
		newUserMiner = append(newUserMiner, userMiner)
	}
	miners, err := g.userMinerRepo.CreateUserMiner(ctx, newUserMiner) //这里是直接使用了gorm的更新操作，所以原来插入操作逻辑不用管
	if err != nil {
		return
	}
	err = tx.Commit().Error
	if err != nil {
		return
	}
	return miners, nil
}

// 用于！！在savegroup时候有新增的节点
func (g *GroupBiz) saveUserGroupNewMiners(ctx context.Context, userID int64, groupID int64, minerInfos []*pro.MinerInfo) (result []*pro.MinerInfo, err error) {
	var newUserMiner []*propo.UserMiner
	mapping := map[string]struct{}{}
	for _, minerInfo := range minerInfos {
		if _, ok := mapping[minerInfo.MinerID.Address()]; ok {
			continue
		}
		mapping[minerInfo.MinerID.Address()] = struct{}{}
		userMiner := &propo.UserMiner{
			UserID:   userID,
			GroupID:  nil,
			MinerID:  minerInfo.MinerID,
			MinerTag: minerInfo.MinerTag,
		}
		if groupID > 0 {
			userMiner.GroupID = &groupID
		}
		newUserMiner = append(newUserMiner, userMiner)
	}
	miners, err := g.userMinerRepo.CreateUserMiner(ctx, newUserMiner)
	if err != nil {
		return
	}
	result = miners
	return
}

func (g *GroupBiz) CountUserMiners(ctx context.Context, req pro.CountUserMinersRequest) (resp pro.CountUserMinersResponse, err error) {
	b := bearer.UseBearer(ctx)
	v := vip.UseVIP(ctx)
	count, err := g.userMinerRepo.CountUserMiners(ctx, b.Id)
	if err != nil {
		return
	}
	maxMinersCount := VipMaxMinersCount(v.MType)
	resp.MaxMinersCount = maxMinersCount
	resp.MinersCount = minn(count, maxMinersCount)
	return
}

func minn(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func (g *GroupBiz) SaveUserMiners(ctx context.Context, req pro.SaveUserMinersRequest) (resp pro.SaveUserMinersResponse, err error) {
	b := bearer.UseBearer(ctx)
	miners, err := g.saveUserGroupMiners(ctx, b.Id, req.GroupID, req.MinersInfo)
	if err != nil {
		return
	}
	resp.GroupID = req.GroupID
	resp.MinersInfo = miners
	return
}

func (g *GroupBiz) DeleteUserMiners(ctx context.Context, req pro.DeleteUserMinersRequest) (resp pro.DeleteUserMinersResponse, err error) {
	//IsUserMiner, err := g.checkUserMiners(ctx, req.MinerID)
	//if err != nil {
	//	return
	//}
	//if IsUserMiner == false {
	//	return
	//}
	//
	//var MinerIDList []chain.SmartAddress
	//MinerIDList = append(MinerIDList, req.MinerID)
	//minerID, err := g.userMinerRepo.DeleteUserMinerList(ctx, req.GroupID, MinerIDList)
	//if err != nil {
	//	return
	//}
	//if minerID != nil {
	//	resp.MinerID = minerID[0]
	//}
	return
}

func (g *GroupBiz) GetGroupNodes(ctx context.Context, req pro.GetGroupNodesRequest) (resp []pro.GetGroupNodesResponseNode, err error) {

	b := bearer.UseBearer(ctx)

	nodes, err := g.groupRepo.GetUserGroupNodes(ctx, b.Id, req.GroupId)
	if err != nil {
		return
	}

	for _, v := range nodes {
		resp = append(resp, pro.GetGroupNodesResponseNode{
			Tag:   v.MinerTag,
			Miner: v.MinerID.Address(),
		})
	}

	return
}

func VipMaxMinersCount(t pro.MemberShipType) int64 {
	switch t {
	case pro.EnterpriseVIP:
		return 30
	case pro.EnterpriseProVIP:
		return math.MaxInt64 //适配现有代码，节点数量无上限
	default:
		return 10
	}
}
