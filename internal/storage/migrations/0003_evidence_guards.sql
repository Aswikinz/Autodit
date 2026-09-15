alter table exception_event add column sequence bigint generated always as identity;
create unique index exception_event_order on exception_event(tenant_id,exception_key,sequence);
create function guard_run_payload() returns trigger language plpgsql as $$
begin
 if new.tenant_id<>old.tenant_id or new.id<>old.id or new.source_id<>old.source_id
 or new.period_id<>old.period_id or new.input<>old.input or new.input_hash<>old.input_hash
 or new.parameter_set_hash<>old.parameter_set_hash or new.rule_manifest<>old.rule_manifest
 or new.submitted_by<>old.submitted_by or new.created_at<>old.created_at then
  raise exception 'run payload is immutable';
 end if;
 return new;
end $$;
create trigger immutable_run_payload before update on audit_run for each row execute function guard_run_payload();
create function guard_exception_evidence() returns trigger language plpgsql as $$
begin
 if row(new.tenant_id,new.key,new.rule_id,new.entity_type,new.entity_id,new.period_id,
 new.rule_version,new.parameter_set_hash,new.snapshot_id,new.engine_version,new.input_hash,new.trace_ref,new.first_seen_at)
 is distinct from row(old.tenant_id,old.key,old.rule_id,old.entity_type,old.entity_id,old.period_id,
 old.rule_version,old.parameter_set_hash,old.snapshot_id,old.engine_version,old.input_hash,old.trace_ref,old.first_seen_at) then
  raise exception 'original exception evidence is immutable';
 end if;
 return new;
end $$;
create trigger immutable_original_evidence before update on exception for each row execute function guard_exception_evidence();
insert into schema_version(version) values(3);
