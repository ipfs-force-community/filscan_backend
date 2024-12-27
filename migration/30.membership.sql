CREATE TABLE pro.membership (
                                ID serial PRIMARY KEY,
                                user_id INTEGER CONSTRAINT rules_users_id_fk REFERENCES pro.users ON UPDATE CASCADE ON DELETE CASCADE,
                                mem_type VARCHAR ( 32 ),
-- 	第一次充值的时间
                                expired_time TIMESTAMP WITH TIME ZONE,
                                created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                                updated_at TIMESTAMP WITH TIME ZONE
);

CREATE TABLE pro.recharge_record (
                                     ID serial PRIMARY KEY,
                                     user_id INTEGER CONSTRAINT rules_users_id_fk REFERENCES pro.users ON UPDATE CASCADE ON DELETE CASCADE,
                                     mem_type VARCHAR ( 32 ),
                                     extend_time VARCHAR ( 16 ),
                                     created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX membership_user ON pro.membership (user_id);
