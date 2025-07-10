package po

type DealProposalPo struct {
	Epoch  int64
	DealID uint64
	Cid    string
}

func (m DealProposalPo) TableName() string {
	return "chain.deal_proposals"
}
