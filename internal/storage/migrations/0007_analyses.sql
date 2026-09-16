create table analysis_version (
 tenant_id uuid not null references tenant(id), id uuid not null,
 revision integer not null check(revision > 0), name text not null,
 dataset jsonb not null, selected_columns jsonb not null, model jsonb not null,
 origin text not null default '', saved_by text not null,
 updated_at timestamptz not null default now(),
 primary key(tenant_id,id,revision)
);
create index analysis_recent on analysis_version(tenant_id,updated_at desc);
alter table analysis_version enable row level security;
alter table analysis_version force row level security;
create policy tenant_isolation on analysis_version
 using(tenant_id::text=current_setting('app.tenant_id',true))
 with check(tenant_id::text=current_setting('app.tenant_id',true));
create trigger immutable before update or delete on analysis_version
 for each row execute function reject_evidence_mutation();
insert into schema_version(version) values(7);
