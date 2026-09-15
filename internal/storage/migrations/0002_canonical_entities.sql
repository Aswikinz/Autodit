-- The first three entities share one constrained ingestion table. The views
-- retain explicit columns and security-invoker semantics (PostgreSQL 15+).
create view payment with (security_invoker=true) as
select tenant_id,source_system_id,source_record_id,snapshot_id,period_id,ingested_at,
posting_date as payment_date,amount,currency_code,reporting_amount,reporting_currency_code,
exchange_rate,rate_date,vendor_id,reversal,intercompany
from canonical_record where entity_type='payment';
create view journal_entry with (security_invoker=true) as
select tenant_id,source_system_id,source_record_id,snapshot_id,period_id,ingested_at,
posting_date,amount,currency_code,reporting_amount,reporting_currency_code,
exchange_rate,rate_date,debit,credit
from canonical_record where entity_type='journal_entry';
create view approval with (security_invoker=true) as
select tenant_id,source_system_id,source_record_id,snapshot_id,period_id,ingested_at,
posting_date as approval_date,amount,currency_code,reporting_amount,reporting_currency_code,
exchange_rate,rate_date,approval_limit
from canonical_record where entity_type='approval';
create table invoice (
 tenant_id uuid not null, source_system_id text not null, source_record_id text not null,
 snapshot_id uuid not null, period_id text not null, ingested_at timestamptz not null,
 document_date date not null, amount numeric(20,4) not null,currency_code char(3) not null,
 reporting_amount numeric(20,4) not null,reporting_currency_code char(3) not null,
 exchange_rate numeric(20,4) not null,rate_date date not null, direction text not null check(direction in ('AP','AR')),
 primary key(tenant_id,source_system_id,source_record_id,snapshot_id),
 foreign key(tenant_id,snapshot_id) references snapshot(tenant_id,id)
);
create table vendor (
 tenant_id uuid not null, source_system_id text not null, source_record_id text not null,
 snapshot_id uuid not null, period_id text not null, ingested_at timestamptz not null,
 valid_from timestamptz not null, valid_to timestamptz, is_current boolean not null,
 name text not null, bank_account_hash text not null,
 check(valid_to is null or valid_to>valid_from), check(is_current=(valid_to is null)),
 primary key(tenant_id,source_system_id,source_record_id,snapshot_id),
 foreign key(tenant_id,snapshot_id) references snapshot(tenant_id,id)
);
create table employee (like vendor including all);
alter table employee add foreign key(tenant_id,snapshot_id) references snapshot(tenant_id,id);
create table access_grant (
 tenant_id uuid not null, source_system_id text not null, source_record_id text not null,
 snapshot_id uuid not null, period_id text not null, ingested_at timestamptz not null,
 principal_id text not null, permission text not null, granted_at timestamptz not null,
 revoked_at timestamptz, primary key(tenant_id,source_system_id,source_record_id,snapshot_id),
 foreign key(tenant_id,snapshot_id) references snapshot(tenant_id,id)
);
do $$ declare t text; begin
 foreach t in array array['invoice','vendor','employee','access_grant'] loop
  execute format('alter table %I enable row level security',t);
  execute format('alter table %I force row level security',t);
  execute format('create policy tenant_isolation on %I using (tenant_id::text = current_setting(''app.tenant_id'',true)) with check (tenant_id::text = current_setting(''app.tenant_id'',true))',t);
  execute format('create trigger immutable before update or delete on %I for each row execute function reject_evidence_mutation()',t);
 end loop;
end $$;
insert into schema_version(version) values(2);
