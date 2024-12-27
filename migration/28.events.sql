CREATE TABLE public.events
(
    id serial primary key,
    image_url varchar(255),
    jump_url varchar(255),
    start_at timestamp with time zone,
    end_at timestamp with time zone
);

CREATE TABLE public.invite_code
(
    id serial primary key,
    code varchar(255),
    user_id integer
        constraint ic_users_id_fk
            references pro.users
            on update cascade on delete cascade
);

create unique index uniq_ic_user_id
    on public.invite_code (user_id);

create unique index uniq_ic_code
    on public.invite_code (code);

CREATE TABLE public.user_invite_record
(
      id serial primary key,
      user_id integer
        constraint uir_users_id_fk
            references pro.users
            on update cascade on delete cascade,
      code varchar(255)
        constraint uir_code_fk
            references public.invite_code(code)
            on update cascade on delete cascade,
      register_time timestamp with time zone,
      user_email varchar(255)
);

create unique index uniq_uir_user_id
    on public.user_invite_record (user_id);

comment on column public.user_invite_record.user_id is '这里的userid表示x用户使用这个邀请码注册了';

ALTER TABLE public.user_invite_record add is_valid boolean;

CREATE TABLE public.invite_success_records
(
     id serial primary key,
     user_id integer
        constraint uir_users_id_fk
            references pro.users
            on update cascade on delete cascade,
     complete_time timestamp with time zone
);

ALTER TABLE public.events add name varchar(500);
