package auth

import(
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Aswikinz/Autodit/internal/platform"
	"github.com/coreos/go-oidc/v3/oidc"
)

func TestOIDCCodeFlow(t *testing.T){key,e:=rsa.GenerateKey(rand.Reader,2048);if e!=nil{t.Fatal(e)};issuer:="";nonce:="";tenant:="11111111-1111-4111-8111-111111111111";subjectTenant:=tenant
	encode:=func(v any)string{b,_:=json.Marshal(v);return base64.RawURLEncoding.EncodeToString(b)}
	provider:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Header().Set("Content-Type","application/json");switch r.URL.Path{case "/.well-known/openid-configuration":_=json.NewEncoder(w).Encode(map[string]any{"issuer":issuer,"authorization_endpoint":issuer+"/authorize","token_endpoint":issuer+"/token","jwks_uri":issuer+"/keys","response_types_supported":[]string{"code"},"subject_types_supported":[]string{"public"},"id_token_signing_alg_values_supported":[]string{"RS256"}})
	case "/keys":_=json.NewEncoder(w).Encode(map[string]any{"keys":[]any{map[string]any{"kty":"RSA","kid":"test","use":"sig","alg":"RS256","n":base64.RawURLEncoding.EncodeToString(key.N.Bytes()),"e":base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
	case "/token":if e:=r.ParseForm();e!=nil||r.Form.Get("code_verifier")==""{http.Error(w,"PKCE required",400);return};unsigned:=encode(map[string]any{"alg":"RS256","kid":"test","typ":"JWT"})+"."+encode(map[string]any{"iss":issuer,"sub":"auditor-1","aud":"autodit","exp":time.Now().Add(time.Hour).Unix(),"iat":time.Now().Unix(),"nonce":nonce,"autodit_tenant":subjectTenant,"autodit_roles":[]string{"auditor"}});digest:=sha256.Sum256([]byte(unsigned));sig,e:=rsa.SignPKCS1v15(rand.Reader,key,crypto.SHA256,digest[:]);if e!=nil{t.Error(e)};_=json.NewEncoder(w).Encode(map[string]any{"access_token":"fixture-access-token","token_type":"Bearer","id_token":unsigned+"."+base64.RawURLEncoding.EncodeToString(sig)})
	default:http.NotFound(w,r)}}));defer provider.Close();issuer=provider.URL;ctx:=oidc.ClientContext(context.Background(),provider.Client());cfg:=platform.Config{AuthMode:"oidc",PublicURL:"https://audit.example",Issuer:issuer,ClientID:"autodit",TenantID:tenant};m,e:=New(ctx,cfg);if e!=nil{t.Fatal(e)}
	flow:=func(want int){t.Helper();w:=httptest.NewRecorder();m.Login(w,httptest.NewRequest("GET","https://audit.example/auth/login",nil));if w.Code!=302{t.Fatal("authorization redirect failed")};location,_:=url.Parse(w.Header().Get("Location"));nonce=location.Query().Get("nonce");if location.Query().Get("code_challenge")==""||nonce==""{t.Fatal("PKCE or nonce missing")};state:=location.Query().Get("state");req:=httptest.NewRequest("GET","https://audit.example/auth/callback?state="+state+"&code=valid",nil).WithContext(ctx);req.AddCookie(w.Result().Cookies()[0]);response:=httptest.NewRecorder();m.Callback(response,req);if response.Code!=want{t.Fatalf("callback status=%d wanted=%d body=%s",response.Code,want,response.Body.String())};if want==303{cookies:=response.Result().Cookies();var sessionCookie *http.Cookie;for _,c:=range cookies{if c.Name=="autodit_session"{sessionCookie=c}};if sessionCookie==nil||!sessionCookie.Secure||!sessionCookie.HttpOnly{t.Fatal("insecure session cookie")};check:=httptest.NewRequest("GET","/api/session",nil);check.AddCookie(sessionCookie);identity,ok:=m.Current(check);if !ok||identity.Subject!="auditor-1"||!Allowed(identity,"exceptions.write")||Allowed(identity,"rules.write"){t.Fatal("claims were not correctly bound to permissions")}}
		response=httptest.NewRecorder();m.Callback(response,req);if response.Code!=401{t.Fatal("authorization code state replay accepted")}}
	flow(303);subjectTenant="22222222-2222-4222-8222-222222222222";flow(403)
	w:=httptest.NewRecorder();m.Callback(w,httptest.NewRequest("GET","/auth/callback?state=invalid",nil));if w.Code!=401{t.Fatal("missing state cookie accepted")}
}
