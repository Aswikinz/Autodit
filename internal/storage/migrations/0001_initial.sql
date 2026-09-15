create table schema_version (version integer primary key, applied_at timestamptz not null default now());
create table tenant (id uuid primary key, name text not null);
create table source (
 tenant_id uuid not null references tenant(id), id text not null, name text not null,
 kind text not null default 'file' check (kind in ('file','postgres','rest')),
 mapping jsonb not null default '{}', interval_minutes integer not null default 1440 check(interval_minutes>=1),
 last_landed_at timestamptz, watermark text not null default '', created_at timestamptz not null default now(),
 primary key(tenant_id,id)
);
create table fiscal_period (
 tenant_id uuid not null references tenant(id), id text not null, start_date date not null, end_date date not null,
 check(end_date>=start_date), primary key(tenant_id,id)
);
create table parameter_set (
 tenant_id uuid not null references tenant(id), hash text not null, content jsonb not null,
 created_at timestamptz not null default now(), primary key(tenant_id,hash)
);
create table rule_version (
 rule_id text not null, version text not null, model jsonb not null, title text not null,
 created_at timestamptz not null default now(), primary key(rule_id,version)
);
create table rule_release (
 tenant_id uuid not null references tenant(id), rule_id text not null, version text not null,
 enabled boolean not null default true, revision integer not null default 1,
 released_by text not null, released_at timestamptz not null default now(),
 primary key(tenant_id,rule_id), foreign key(rule_id,version) references rule_version(rule_id,version)
);
create table tenant_setting (
 tenant_id uuid primary key references tenant(id), parameter_hash text not null,
 revision integer not null default 1, foreign key(tenant_id,parameter_hash) references parameter_set(tenant_id,hash)
);
create table audit_run (
 tenant_id uuid not null references tenant(id), id uuid not null, source_id text not null, period_id text not null,
 status text not null check(status in ('queued','running','completed','tieout_failed','failed','ceiling_exceeded')),
 stage text not null default 'queued', input jsonb not null, input_hash text not null,
 parameter_set_hash text not null, rule_manifest jsonb not null, submitted_by text not null,
 snapshot_id uuid, differences jsonb not null default '[]', error_code text not null default '',
 record_count integer not null default 0, exception_count integer not null default 0,
 created_at timestamptz not null default now(), completed_at timestamptz,
 primary key(tenant_id,id), foreign key(tenant_id,source_id) references source(tenant_id,id),
 foreign key(tenant_id,period_id) references fiscal_period(tenant_id,id),
 foreign key(tenant_id,parameter_set_hash) references parameter_set(tenant_id,hash)
);
create index audit_run_pending on audit_run(created_at) where status='queued';
create index audit_run_scope on audit_run(tenant_id,source_id,period_id,created_at);
create table snapshot (
 tenant_id uuid not null, id uuid not null, run_id uuid not null, object_ref text not null,
 content_hash text not null, record_count integer not null, created_at timestamptz not null default now(),
 primary key(tenant_id,id), foreign key(tenant_id,run_id) references audit_run(tenant_id,id)
);
create table canonical_record (
 tenant_id uuid not null, source_system_id text not null, source_record_id text not null,
 snapshot_id uuid not null, period_id text not null, ingested_at timestamptz not null default now(),
 entity_type text not null, posting_date date not null, amount numeric(20,4) not null,
 currency_code char(3) not null, reporting_amount numeric(20,4) not null, reporting_currency_code char(3) not null,
 exchange_rate numeric(20,4) not null, rate_date date not null, vendor_id text not null,
 debit numeric(20,4) not null, credit numeric(20,4) not null, approval_limit numeric(20,4) not null,
 reversal boolean not null, intercompany boolean not null,
 primary key(tenant_id,source_system_id,source_record_id,snapshot_id,entity_type),
 foreign key(tenant_id,snapshot_id) references snapshot(tenant_id,id)
);
create index canonical_population on canonical_record(tenant_id,snapshot_id,entity_type,vendor_id,currency_code,amount,posting_date);
create table exception (
 tenant_id uuid not null, key text not null, source_id text not null, period_id text not null,
 rule_id text not null, entity_type text not null, entity_id text not null,
 state text not null check(state in ('open','in_review','accepted','dismissed','suppressed','reopened','resolved_in_source')),
 severity text not null check(severity in ('low','medium','high','critical')), owner_role text not null,
 rule_version text not null, parameter_set_hash text not null, snapshot_id uuid not null,
 engine_version text not null, input_hash text not null, trace_ref text not null,
 latest_input_hash text not null, suppression_until timestamptz, revision integer not null default 1,
 first_seen_at timestamptz not null default now(), last_seen_at timestamptz not null default now(),
 primary key(tenant_id,key), foreign key(rule_id,rule_version) references rule_version(rule_id,version),
 foreign key(tenant_id,parameter_set_hash) references parameter_set(tenant_id,hash),
 foreign key(tenant_id,snapshot_id) references snapshot(tenant_id,id)
);
create index exception_queue on exception(tenant_id,state,severity,first_seen_at,key);
create index exception_scope on exception(tenant_id,source_id,period_id,rule_id);
create table observation (
 tenant_id uuid not null, id uuid not null, exception_key text not null, run_id uuid not null,
 rule_id text not null, rule_version text not null, parameter_set_hash text not null,
 snapshot_id uuid not null, engine_version text not null, input_hash text not null,
 trace_ref text not null, input jsonb not null, result jsonb not null, trace jsonb not null,
 classification text not null, created_at timestamptz not null default now(),
 primary key(tenant_id,id), unique(tenant_id,run_id,exception_key),
 foreign key(tenant_id,exception_key) references exception(tenant_id,key),
 foreign key(tenant_id,run_id) references audit_run(tenant_id,id),
 foreign key(rule_id,rule_version) references rule_version(rule_id,version),
 foreign key(tenant_id,snapshot_id) references snapshot(tenant_id,id),
 foreign key(tenant_id,parameter_set_hash) references parameter_set(tenant_id,hash)
);
create table exception_event (
 tenant_id uuid not null, id uuid not null, exception_key text not null, actor text not null,
 action text not null, reason text not null, from_state text not null, to_state text not null,
 previous_hash text not null, event_hash text not null, created_at timestamptz not null,
 primary key(tenant_id,id), foreign key(tenant_id,exception_key) references exception(tenant_id,key)
);
create table audit_event (
 tenant_id uuid not null references tenant(id), id uuid not null, actor text not null,
 action text not null, target_id text not null, created_at timestamptz not null default now(),
 primary key(tenant_id,id)
);
create table run_stage (
 tenant_id uuid not null, id uuid not null, run_id uuid not null, stage text not null,
 status text not null, created_at timestamptz not null default now(),
 primary key(tenant_id,id), foreign key(tenant_id,run_id) references audit_run(tenant_id,id)
);
create function reject_evidence_mutation() returns trigger language plpgsql as $$
begin raise exception 'audit evidence is immutable'; end $$;
do $$ declare t text; begin
 foreach t in array array['snapshot','canonical_record','observation','exception_event','audit_event','run_stage','rule_version','parameter_set'] loop
  execute format('create trigger immutable before update or delete on %I for each row execute function reject_evidence_mutation()',t);
 end loop;
 foreach t in array array['source','fiscal_period','parameter_set','rule_release','tenant_setting','audit_run','snapshot','canonical_record','exception','observation','exception_event','audit_event','run_stage'] loop
  execute format('alter table %I enable row level security',t);
  execute format('alter table %I force row level security',t);
  execute format('create policy tenant_isolation on %I using (tenant_id::text = current_setting(''app.tenant_id'',true)) with check (tenant_id::text = current_setting(''app.tenant_id'',true))',t);
 end loop;
end $$;
insert into schema_version(version) values(1);
