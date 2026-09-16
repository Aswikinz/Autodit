"""Single source of truth for the public JSON API and generated browser client types."""
import json
from pathlib import Path

obj = {'type': 'object', 'additionalProperties': True}
schemas = {
 'Identity': {'type':'object','required':['subject','tenant_id','roles','csrf_token'],'properties': {'subject':{'type':'string'},'tenant_id':{'type':'string'},'roles':{'type':'array','items':{'type':'string'}},'csrf_token':{'type':'string'}}},
 'Session': {'type':'object','required':['authenticated','identity','mode','login_url'],'properties': {'authenticated':{'type':'boolean'},'identity':{'$ref':'#/components/schemas/Identity'},'mode':{'type':'string'},'login_url':{'type':'string'}}},
 'Object':obj, 'ObjectList':{'type':'array','items':obj},
}
paths = {}
endpoints = [
 ('post','/api/sources/connection','Object','Object'),('post','/api/sources/preview','Object','Object'),('post','/api/sources/workbook','Object','Object'),
 ('get','/api/workspace','Object',None),('put','/api/admin/settings',None,'Object'),
 ('post','/api/password-login',None,'Object'),('post','/api/password',None,'Object'),
 ('get','/api/admin/users','Object',None),('post','/api/admin/users',None,'Object'),
 ('get','/api/session','Session',None),('post','/api/login',None,'Object'),('post','/api/logout',None,None),
 ('get','/api/exceptions','Object',None),('get','/api/exceptions/{key}','Object',None),
 ('post','/api/exceptions/{key}/disposition',None,'Object'),('post','/api/exceptions/{key}/comments',None,'Object'),
 ('post','/api/observations/{id}/replay','Object',None),('get','/api/runs','ObjectList',None),
 ('get','/api/runs/{id}','Object',None),('post','/api/runs','Object','Object'),('post','/api/imports','Object','Object'),
 ('get','/api/sources','ObjectList',None),('post','/api/sources',None,'Object'),('post','/api/sources/profile','Object','Object'),
 ('get','/api/rules','ObjectList',None),('put','/api/rules/{id}',None,'Object'),('post','/api/rules/simulate','Object','Object'),
 ('get','/api/parameters','Object',None),('put','/api/parameters',None,'Object'),('get','/api/assurance','Object',None),
]
for method, route, response, body in endpoints:
    code = '204' if response is None else ('202' if method=='post' and route in ('/api/runs','/api/imports') else '200')
    operation = {'responses': {code: {'description':'Success'}, 'default': {'description':'Error','content':{'application/json':{'schema':obj}}}}, 'security':[{'sessionCookie':[]}]}
    if response: operation['responses'][code]['content']={'application/json':{'schema':{'$ref':f'#/components/schemas/{response}'}}}
    if body: operation['requestBody']={'required':True,'content':{'application/json':{'schema':{'$ref':f'#/components/schemas/{body}'}}}}
    params=[]
    for param in ('key','id'):
        if '{'+param+'}' in route: params.append({'name':param,'in':'path','required':True,'schema':{'type':'string'}})
    if route=='/api/exceptions':
        params += [{'name':p,'in':'query','schema':{'type':'integer' if p in ('page','size') else 'string'}} for p in ('page','size','state','rule','severity','search','sort')]
    if method not in ('get',): params.append({'name':'X-CSRF-Token','in':'header','schema':{'type':'string'}})
    if params: operation['parameters']=params
    paths.setdefault(route,{})[method]=operation
spec={'openapi':'3.0.3','info':{'title':'Autodit API','version':'0.1.0','description':'Tenant scope derives exclusively from the authenticated session. Monetary values use decimal strings. See docs/user/api.md for contracts.'},'paths':paths,'components':{'schemas':schemas,'securitySchemes':{'sessionCookie':{'type':'apiKey','in':'cookie','name':'autodit_session'}}}}
target=Path(__file__).resolve().parent.parent/'internal/api/openapi.json'
target.write_text(json.dumps(spec,indent=2)+'\n',encoding='utf-8')
