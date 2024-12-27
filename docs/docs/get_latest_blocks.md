## 获取某区块详情
<a id=获取某区块详情> </a>

### 基本信息

**Path：** /api/v1/BlockDetails

**Method：** POST

**接口描述：**

<p>区块链页面某区块详情信息接口；<br></p>


### 请求参数
**Headers**

| 参数名称     | 参数值           | 是否必须 | 示例 | 备注 |
| ------------ | ---------------- | -------- | ---- | ---- |
| Content-Type | application/json | 是       |      |      |

**Body**

<table>
  <thead class="ant-table-thead">
    <tr>
      <th key=name>名称</th><th key=type>类型</th><th key=required>是否必须</th><th key=default>默认值</th><th key=desc>备注</th><th key=sub>其他信息</th>
    </tr>
  </thead><tbody className="ant-table-tbody"><tr key=0-0><td key=0><span style="padding-left: 0px"><span style="color: #8c8a8a"></span> block_cid</span></td><td key=1><span>string</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">区块cid</span></td><td key=5></td></tr>
               </tbody>
              </table>

### 返回数据

<table>
  <thead class="ant-table-thead">
    <tr>
      <th key=name>名称</th><th key=type>类型</th><th key=required>是否必须</th><th key=default>默认值</th><th key=desc>备注</th><th key=sub>其他信息</th>
    </tr>
  </thead><tbody className="ant-table-tbody"><tr key=0-0><td key=0><span style="padding-left: 0px"><span style="color: #8c8a8a"></span> result</span></td><td key=1><span>object</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap"></span></td><td key=5></td></tr><tr key=0-0-0><td key=0><span style="padding-left: 20px"><span style="color: #8c8a8a">├─</span> block_details</span></td><td key=1><span>object</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap"></span></td><td key=5></td></tr><tr key=0-0-0-0><td key=0><span style="padding-left: 40px"><span style="color: #8c8a8a">├─</span> block_basic</span></td><td key=1><span>object</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">区块基础信息</span></td><td key=5></td></tr><tr key=0-0-0-0-0><td key=0><span style="padding-left: 60px"><span style="color: #8c8a8a">├─</span> height</span></td><td key=1><span>number</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">当前高度</span></td><td key=5></td></tr><tr key=0-0-0-0-1><td key=0><span style="padding-left: 60px"><span style="color: #8c8a8a">├─</span> cid</span></td><td key=1><span>string</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">当前区块cid</span></td><td key=5></td></tr><tr key=0-0-0-0-2><td key=0><span style="padding-left: 60px"><span style="color: #8c8a8a">├─</span> block_time</span></td><td key=1><span>number</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">当前区块时间（需要前端根据区块计算时间）</span></td><td key=5></td></tr><tr key=0-0-0-0-3><td key=0><span style="padding-left: 60px"><span style="color: #8c8a8a">├─</span> miner_id</span></td><td key=1><span>string</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">赢票节点号</span></td><td key=5></td></tr><tr key=0-0-0-0-4><td key=0><span style="padding-left: 60px"><span style="color: #8c8a8a">├─</span> messages_count</span></td><td key=1><span>number</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">区块消息数</span></td><td key=5></td></tr><tr key=0-0-0-0-5><td key=0><span style="padding-left: 60px"><span style="color: #8c8a8a">├─</span> reward</span></td><td key=1><span>string</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">奖励</span></td><td key=5></td></tr><tr key=0-0-0-0-6><td key=0><span style="padding-left: 60px"><span style="color: #8c8a8a">├─</span> mined_reward</span></td><td key=1><span>string</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">区块奖励</span></td><td key=5></td></tr><tr key=0-0-0-0-7><td key=0><span style="padding-left: 60px"><span style="color: #8c8a8a">├─</span> tx_fee_reward</span></td><td key=1><span>string</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">区块奖励手续费</span></td><td key=5></td></tr><tr key=0-0-0-1><td key=0><span style="padding-left: 40px"><span style="color: #8c8a8a">├─</span> win_count</span></td><td key=1><span>number</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">赢票数</span></td><td key=5></td></tr><tr key=0-0-0-2><td key=0><span style="padding-left: 40px"><span style="color: #8c8a8a">├─</span> parents</span></td><td key=1><span>string []</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">父区块cid列表(上一高度的赢票区块cid列表)</span></td><td key=5><p key=3><span style="font-weight: '700'">item 类型: </span><span>string</span></p></td></tr><tr key=array-10><td key=0><span style="padding-left: 60px"><span style="color: #8c8a8a">├─</span> </span></td><td key=1><span></span></td><td key=2>非必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap"></span></td><td key=5></td></tr><tr key=0-0-0-3><td key=0><span style="padding-left: 40px"><span style="color: #8c8a8a">├─</span> parent_weight</span></td><td key=1><span>string</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">父块重量</span></td><td key=5></td></tr><tr key=0-0-0-4><td key=0><span style="padding-left: 40px"><span style="color: #8c8a8a">├─</span> parent_base_fee</span></td><td key=1><span>string</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">父块基础费率</span></td><td key=5></td></tr><tr key=0-0-0-5><td key=0><span style="padding-left: 40px"><span style="color: #8c8a8a">├─</span> ticket_value</span></td><td key=1><span>string</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">赢票值</span></td><td key=5></td></tr><tr key=0-0-0-6><td key=0><span style="padding-left: 40px"><span style="color: #8c8a8a">├─</span> state_root</span></td><td key=1><span>string</span></td><td key=2>必须</td><td key=3></td><td key=4><span style="white-space: pre-wrap">根</span></td><td key=5></td></tr>
               </tbody>
              </table>
