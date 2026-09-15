-- AP-01 Duplicate payments. Population: full period, excluding reversals and
-- intercompany. Blocking key: vendor, currency, exact amount, 30-day window.
-- Engineering owns the pairing; auditors can change materiality and routing.
select a.source_record_id, b.source_record_id,
       a.amount::text, a.currency_code, a.posting_date::text, b.posting_date::text
from canonical_record a
join canonical_record b on a.tenant_id=b.tenant_id
 and a.snapshot_id=b.snapshot_id and a.source_system_id=b.source_system_id
 and a.vendor_id=b.vendor_id and a.currency_code=b.currency_code
 and a.amount=b.amount and a.source_record_id<b.source_record_id
 and b.posting_date between a.posting_date-30 and a.posting_date+30
where a.tenant_id=$1 and a.snapshot_id=$2
 and a.entity_type='payment' and b.entity_type='payment'
 and not a.reversal and not b.reversal and not a.intercompany and not b.intercompany
order by a.source_record_id,b.source_record_id
limit $3;
