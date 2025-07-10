package pro

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type GroupAPI interface {
	GetUserGroups(ctx context.Context, req GetUserGroupsRequest) (resp GetUserGroupsResponse, err error)
	GetGroup(ctx context.Context, req GetGroupRequest) (resp GetGroupResponse, err error)
	DeleteGroup(ctx context.Context, req DeleteGroupRequest) (resp DeleteGroupResponse, err error)
	CountUserMiners(ctx context.Context, req CountUserMinersRequest) (resp CountUserMinersResponse, err error)
	SaveGroupMiners(ctx context.Context, req SaveGroupMinersRequest) (resp SaveGroupMinersResponse, err error)
	DeleteGroupMiners(ctx context.Context, req DeleteGroupMinersRequest) (resp DeleteGroupMinersResponse, err error)
	SaveUserMiners(ctx context.Context, req SaveUserMinersRequest) (resp SaveUserMinersResponse, err error)
	DeleteUserMiners(ctx context.Context, req DeleteUserMinersRequest) (resp DeleteUserMinersResponse, err error)
	GetGroupNodes(ctx context.Context, req GetGroupNodesRequest) (resp []GetGroupNodesResponseNode, err error)
}

type GetUserGroupsRequest struct {
}

type GetUserGroupsResponse struct {
	GroupInfoList []*GroupInfo `json:"group_info_list"`
}

type GroupInfo struct {
	GroupID    int64        `json:"group_id"`
	GroupName  string       `json:"group_name"`
	IsDefault  bool         `json:"is_default"`
	MinersInfo []*MinerInfo `json:"miners_info"`
}

type GetGroupRequest struct {
}

type GetGroupResponse struct {
	GroupList []*GroupName `json:"group_list"`
}

type GroupName struct {
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	IsDefault bool   `json:"is_default"`
}

type DeleteGroupRequest struct {
	GroupID int64 `json:"group_id"`
}

type DeleteGroupResponse struct {
	//GroupID int64 `json:"group_id"`
}

type CountUserMinersRequest struct {
}

type CountUserMinersResponse struct {
	MinersCount    int64 `json:"miners_count"`
	MaxMinersCount int64 `json:"max_miners_count"`
}

type SaveGroupMinersRequest struct {
	GroupID    int64        `json:"group_id"`
	GroupName  string       `json:"group_name"`
	IsDefault  bool         `json:"is_default"`
	MinerInfos []*MinerInfo `json:"miners_info,omitempty"`
}

type SaveGroupMinersResponse struct {
	GroupID    int64        `json:"group_id"`
	MinersInfo []*MinerInfo `json:"miners_info"`
}

type DeleteGroupMinersRequest struct {
	MinerID string `json:"miner_id"`
}

type DeleteGroupMinersResponse struct {
	MinerID string `json:"miner_id"`
}

type SaveUserMinersRequest struct {
	GroupID    int64        `json:"group_id"`
	MinersInfo []*MinerInfo `json:"miners_info"`
}

type SaveUserMinersResponse struct {
	GroupID    int64        `json:"group_id"`
	MinersInfo []*MinerInfo `json:"miners_info"`
}

type MinerInfo struct {
	MinerID  chain.SmartAddress `json:"miner_id"`
	MinerTag string             `json:"miner_tag"`
}

type DeleteUserMinersRequest struct {
	GroupID int64              `json:"group_id"`
	MinerID chain.SmartAddress `json:"miner_id"`
}

type DeleteUserMinersResponse struct {
	MinerID chain.SmartAddress `json:"miner_id"`
}

type GetGroupNodesRequest struct {
	GroupId int64 `json:"group_id"`
}

type GetGroupNodesResponseNode struct {
	Tag   string `json:"tag"`
	Miner string `json:"miner"`
}
