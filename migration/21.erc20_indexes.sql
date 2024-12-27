CREATE INDEX idx_contract ON fevm.erc_20_transfers ("contract_id","dex");
CREATE INDEX idx_from_to ON fevm.erc_20_transfers ("from", "to");
CREATE INDEX idx_cid ON fevm.erc_20_transfers ("cid");
