-- +migrate Up
alter table device_keys
    add column gen_app_key bytea not null default decode('00000000000000000000000000000000', 'hex');

alter table device_keys
    alter column gen_app_key drop default;

-- +migrate Down
alter table device_keys
    drop column gen_app_key;

