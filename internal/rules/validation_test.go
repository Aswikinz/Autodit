package rules

import("encoding/json";"os";"path/filepath";"testing")

func TestRejectUnsafeModelVariations(t *testing.T){t.Parallel();pack,e:=LoadPack("../../rulepack");if e!=nil{t.Fatal(e)};for _,tc:=range []struct{name string;edit func(map[string]any)}{
	{"duplicate node",func(g map[string]any){nodes:=g["nodes"].([]any);nodes[1].(map[string]any)["id"]="input"}},
	{"wrong edge",func(g map[string]any){g["edges"].([]any)[0].(map[string]any)["targetId"]="output"}},
	{"unbounded hit policy",func(g map[string]any){g["nodes"].([]any)[1].(map[string]any)["content"].(map[string]any)["hitPolicy"]="collect"}},
	{"cross-record input",func(g map[string]any){g["nodes"].([]any)[1].(map[string]any)["content"].(map[string]any)["inputs"].([]any)[0].(map[string]any)["field"]="population"}},
	{"unknown output",func(g map[string]any){g["nodes"].([]any)[1].(map[string]any)["content"].(map[string]any)["outputs"].([]any)[0].(map[string]any)["field"]="unknown"}},
	{"duplicate output",func(g map[string]any){g["nodes"].([]any)[1].(map[string]any)["content"].(map[string]any)["outputs"].([]any)[0].(map[string]any)["field"]="severity"}},
	{"expression condition",func(g map[string]any){g["nodes"].([]any)[1].(map[string]any)["content"].(map[string]any)["rules"].([]any)[0].(map[string]any)["condition"]="some(map)"}},
	{"expression output",func(g map[string]any){g["nodes"].([]any)[1].(map[string]any)["content"].(map[string]any)["rules"].([]any)[0].(map[string]any)["flag"]="1+2"}},
	{"unknown severity",func(g map[string]any){g["nodes"].([]any)[1].(map[string]any)["content"].(map[string]any)["rules"].([]any)[0].(map[string]any)["severity"]=`"unknown"`}},
	{"admin routing",func(g map[string]any){g["nodes"].([]any)[1].(map[string]any)["content"].(map[string]any)["rules"].([]any)[0].(map[string]any)["owner_role"]=`"admin"`}},
	}{t.Run(tc.name,func(t *testing.T){var graph map[string]any;_=json.Unmarshal(pack[0].Content,&graph);tc.edit(graph);b,_:=json.Marshal(graph);if ValidateModel(b)==nil{t.Fatal("unsafe model accepted")}})}
	dir:=t.TempDir();_=os.MkdirAll(filepath.Join(dir,"ap"),0700);_=os.WriteFile(filepath.Join(dir,"ap","AP-01.jdm.json"),[]byte("{}"),0600);if _,e=LoadPack(dir);e==nil{t.Fatal("invalid shipped model accepted")}
	p:=Defaults();p.CurrencyThresholds=map[string]string{"EUR":"1500.1234"};if p.Validate()!=nil{t.Fatal("valid currency policy rejected")};if v,e:=p.MaterialityFor("EUR");e!=nil||v!="1500.1234"{t.Fatal("wrong currency policy")};p.CurrencyThresholds=map[string]string{"bad":"1"};if p.Validate()==nil{t.Fatal("bad currency accepted")};p.CurrencyThresholds=map[string]string{"EUR":"-1"};if p.Validate()==nil{t.Fatal("negative threshold accepted")};if _,e=p.MaterialityFor("LKR");e==nil{t.Fatal("unconfigured currency used implicit threshold")}
}
