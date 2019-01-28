-- +migrate Up
alter table device_keys
    add column gen_app_key bytea not null default decode('00000000000000000000000000000000', 'hex');

alter table device_keys
    alter column gen_app_key drop default;

alter table multicast_group
    add column mc_key bytea not null default decode('00000000000000000000000000000000', 'hex'),
    add column f_cnt bigint not null default 0;

alter table multicast_group
    alter column mc_key drop default,
    alter column f_cnt drop default;

-- +migrate Down
alter table multicast_group
    drop column mc_key,
    drop column f_cnt;

alter table device_keys
    drop column gen_app_key;

