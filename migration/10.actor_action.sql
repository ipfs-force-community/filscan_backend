create table chain.actor_actions
(
    epoch    bigint  not null,
    actor_id varchar not null,
    action   integer,
    constraint actor_actions_pk
        primary key (epoch, actor_id)
);

comment on column chain.actor_actions.action is '1 新增、2 更新';

