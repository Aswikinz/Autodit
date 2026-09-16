create table local_user (
 tenant_id uuid not null references tenant(id), username text not null,
 display_name text not null, password_hash text not null, roles text[] not null,
 enabled boolean not null default true, must_change_password boolean not null default true,
 revision integer not null default 1, created_at timestamptz not null default now(),
 primary key(tenant_id,username)
);
alter table local_user enable row level security;
alter table local_user force row level security;
create policy tenant_isolation on local_user using(tenant_id::text=current_setting('app.tenant_id',true)) with check(tenant_id::text=current_setting('app.tenant_id',true));
create table workspace_setting (
 tenant_id uuid primary key references tenant(id), content jsonb not null,
 revision integer not null default 1
);
alter table workspace_setting enable row level security;
alter table workspace_setting force row level security;
create policy tenant_isolation on workspace_setting using(tenant_id::text=current_setting('app.tenant_id',true)) with check(tenant_id::text=current_setting('app.tenant_id',true));
insert into schema_version(version) values(5);
