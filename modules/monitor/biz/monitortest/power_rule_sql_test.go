package monitortest

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/biz/proutils"

	mdal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/dal"
	prodal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/dal"

	"github.com/gozelle/fs"
	"github.com/gozelle/spew"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
)

func TestGenerateSql(t *testing.T) {
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)

	spew.Json(conf)
	db, _, err := injector.NewGormDB(conf)
	require.NoError(t, err)

	ruleRepo := mdal.NewRuleDal(db)
	groupRepo := prodal.NewGroupDal(db)
	ctx := context.Background()

	var ids []int64
	var mails []string
	err = db.Raw(`SELECT id FROM pro.users order by id`).Scan(&ids).Error
	if err != nil {
		return
	}
	err = db.Raw(`SELECT mail FROM pro.users order by id`).Scan(&mails).Error
	if err != nil {
		return
	}
	fmt.Println(ids)
	fmt.Println(mails)
	if len(ids) != len(mails) {
		return
	}
	cnt := 0
	perCnt := 0
	groupCnt := 0 //要包括默认，如果老会员都没有规则的话，添加的Power rule数量应该等于group cnt数量
	perGroupCnt := 0
	tg, tc := 0, 0
	//现在大家都没有会员，所以让所有的没加的会员默认为f
	str := "INSERT INTO \"monitor\".\"rules\" (\"user_id\", \"group_id\", \"miner_id_or_all\", \"uuid\", \"account_type\", \"account_addr\", \"monitor_type\", \"operator\", \"operand\", \"mail_alert\", \"msg_alert\", \"call_alert\", \"interval\", \"is_active\", \"is_vip\", \"description\") VALUES ($userid, $groupid, '', '$uuid', '', '', 'PowerMonitor', NULL, NULL, '$mail', NULL, NULL, 15, 't', 'f', '1. 扇区发生错误 2. 扇区主动终止 3. 扇区正常到期');"
	for j, userID := range ids {
		userRules, err := ruleRepo.SelectRulesByUserIDAndType(ctx, userID, "PowerMonitor")
		if err != nil {
			return
		}
		groups, err := groupRepo.SelectGroupsByUserID(ctx, userID)
		if err != nil {
			return
		}
		perCnt = 0
		perGroupCnt = len(groups) + 1
		groupCnt += len(groups) + 1
		if len(userRules) != len(groups)+1 {
			groupIDList := make([]int64, len(groups))
			groupIDPowerList := make([]int64, len(userRules))
			for i, group := range groups {
				groupIDList[i] = group.Id
			}
			groupIDList = append(groupIDList, 0)
			for i, rule := range userRules {
				groupIDPowerList[i] = rule.GroupID
			}
			sliceDifference := proutils.SliceDifferenceAMinusB(groupIDList, groupIDPowerList)
			if sliceDifference != nil {
				perCnt = 0
				for _, groupid := range sliceDifference {
					uid := strconv.FormatInt(userID, 10)
					gid := strconv.FormatInt(groupid, 10)
					time.Sleep(50 * time.Millisecond)
					alertUUID, _ := uuid.NewUUID()
					alertUUIDStr := alertUUID.String()
					tmpstr := strings.ReplaceAll(str, "$userid", uid)
					tmpstr = strings.ReplaceAll(tmpstr, "$groupid", gid)
					tmpstr = strings.ReplaceAll(tmpstr, "$uuid", alertUUIDStr)
					tmpstr = strings.ReplaceAll(tmpstr, "$mail", mails[j])
					fmt.Println(tmpstr)
					cnt++
					perCnt++
				}
				if perGroupCnt != perCnt {
					fmt.Println(perGroupCnt, perCnt)
					fmt.Println(sliceDifference)
				}
				tg += perGroupCnt
				tc += perCnt
			}
		}
	}
	fmt.Println(groupCnt, cnt)
	fmt.Println(tg, tc)
}
