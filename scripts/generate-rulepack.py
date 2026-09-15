"""Generate the initial portable, bounded JDM tables; no client data involved."""
import json
from pathlib import Path

root = Path(__file__).resolve().parent.parent
for rule, area, severity in [('AP-01', 'ap', 'high'), ('AP-02', 'ap', 'high'), ('JE-01', 'je', 'medium')]:
    model = {'nodes': [
        {'id': 'input', 'name': 'Candidate', 'type': 'inputNode', 'position': {'x': 50, 'y': 150}},
        {'id': 'decision', 'name': rule, 'type': 'decisionTableNode', 'position': {'x': 350, 'y': 150}, 'content': {
            'hitPolicy': 'first', 'inputs': [{'id': 'condition', 'name': 'Qualifies under tenant policy', 'field': 'qualifies'}],
            'outputs': [{'id': field, 'name': field.replace('_', ' ').title(), 'field': field} for field in ['flag', 'severity', 'owner_role']],
            'rules': [
                {'_id': 'qualifying', 'condition': 'true', 'flag': 'true', 'severity': json.dumps(severity), 'owner_role': '"auditor"'},
                {'_id': 'otherwise', 'condition': '', 'flag': 'false', 'severity': '"low"', 'owner_role': '"auditor"'}]}},
        {'id': 'output', 'name': 'Decision', 'type': 'outputNode', 'position': {'x': 650, 'y': 150}}],
        'edges': [{'id': 'in', 'sourceId': 'input', 'targetId': 'decision'}, {'id': 'out', 'sourceId': 'decision', 'targetId': 'output'}]}
    target = root / 'rulepack' / area / f'{rule}.jdm.json'
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(model, indent=2) + '\n', encoding='utf-8')
