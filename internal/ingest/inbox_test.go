package ingest

import("context";"os";"path/filepath";"testing")

func TestInboxCompletenessAndFailures(t *testing.T){t.Parallel();dir:=t.TempDir();fixture,e:=os.ReadFile("../../test/fixtures/population.json");if e!=nil{t.Fatal(e)};_=os.WriteFile(filepath.Join(dir,"a-invalid.json"),[]byte("broken"),0600);_=os.WriteFile(filepath.Join(dir,"b.partial"),fixture,0600);_=os.WriteFile(filepath.Join(dir,"c-ready.json"),fixture,0600);landed,e:=Inbox(context.Background(),dir);if e==nil||len(landed)!=1||len(landed[0].Receipt)!=64{t.Fatal("one corrupt extract blocked a complete file")};first:=landed[0].Receipt;landed,_=Inbox(context.Background(),dir);if landed[0].Receipt!=first{t.Fatal("unstable receipt")};empty,e:=Inbox(context.Background(),filepath.Join(dir,"absent"));if e!=nil||len(empty)!=0{t.Fatal("missing inbox should be idle")};ctx,cancel:=context.WithCancel(context.Background());cancel();if _,e=Inbox(ctx,dir);e==nil{t.Fatal("cancelled scan continued")}}
