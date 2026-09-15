create table ingestion_receipt (
 tenant_id uuid not null references tenant(id), receipt_hash text not null,
 run_id uuid not null, created_at timestamptz not null default now(),
 primary key(tenant_id,receipt_hash), foreign key(tenant_id,run_id) references audit_run(tenant_id,id)
);
alter table ingestion_receipt enable row level security;
alter table ingestion_receipt force row level security;
create policy tenant_isolation on ingestion_receipt using(tenant_id::text=current_setting('app.tenant_id',true)) with check(tenant_id::text=current_setting('app.tenant_id',true));
create trigger immutable before update or delete on ingestion_receipt for each row execute function reject_evidence_mutation();
insert into schema_version(version) values(4);
