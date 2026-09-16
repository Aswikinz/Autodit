create table workflow_definition (
 tenant_id uuid not null references tenant(id), revision integer not null,
 content jsonb not null, created_at timestamptz not null default now(),
 primary key(tenant_id,revision)
);
create table review_case (
 tenant_id uuid not null, exception_key text not null, workflow_revision integer not null,
 step_index integer not null default 0, status text not null default 'active' check(status in ('active','closed')),
 assignee text not null default '', last_actor text not null default '', revision integer not null default 1,
 created_at timestamptz not null default now(), updated_at timestamptz not null default now(),
 primary key(tenant_id,exception_key), foreign key(tenant_id,exception_key) references exception(tenant_id,key),
 foreign key(tenant_id,workflow_revision) references workflow_definition(tenant_id,revision)
);
create table case_event (
 tenant_id uuid not null, id uuid not null, exception_key text not null,
 actor text not null, action text not null, note text not null, step_index integer not null,
 assignee text not null default '', sequence bigint generated always as identity,
 created_at timestamptz not null default now(), primary key(tenant_id,id),
 foreign key(tenant_id,exception_key) references review_case(tenant_id,exception_key)
);
do $$ declare tbl text; begin
 foreach tbl in array array['workflow_definition','review_case','case_event'] loop
 execute format('alter table %I enable row level security',tbl);
 execute format('alter table %I force row level security',tbl);
 execute format('create policy tenant_isolation on %I using(tenant_id::text=current_setting(''app.tenant_id'',true)) with check(tenant_id::text=current_setting(''app.tenant_id'',true))',tbl);
 end loop;
end $$;
create trigger immutable before update or delete on workflow_definition for each row execute function reject_evidence_mutation();
create trigger immutable before update or delete on case_event for each row execute function reject_evidence_mutation();
insert into schema_version(version) values(6);
