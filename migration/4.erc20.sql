ALTER TABLE "fevm"."erc_20_transfers"
  ADD COLUMN "dex" varchar(255),
  ADD COLUMN "method" varchar(255),
  ADD COLUMN "token_name" varchar(255);

ALTER TABLE "fevm"."erc_20_transfers"
  ADD COLUMN "decimal" INT Default 0;