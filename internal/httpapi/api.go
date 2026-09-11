package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/thomas-dev7/goflow/internal/auth"
	"github.com/thomas-dev7/goflow/internal/repository"
)

type ctxKey string
const userKey ctxKey = "user_id"

type API struct {
	repo  *repository.Repository
	auth  *auth.Manager
	redis *redis.Client
	log   *zap.Logger
}

func New(repo *repository.Repository, authManager *auth.Manager, redisClient *redis.Client, log *zap.Logger) *API {
	return &API{repo:repo,auth:authManager,redis:redisClient,log:log}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter,r *http.Request){ JSON(w,http.StatusOK,map[string]string{"status":"ok"}) })
	mux.HandleFunc("GET /ready", a.ready)
	mux.HandleFunc("POST /api/v1/auth/register", a.register)
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.Handle("GET /api/v1/products", a.requireAuth(http.HandlerFunc(a.listProducts)))
	mux.Handle("POST /api/v1/products", a.requireAuth(http.HandlerFunc(a.createProduct)))
	mux.Handle("POST /api/v1/orders", a.requireAuth(http.HandlerFunc(a.createOrder)))
	mux.Handle("GET /api/v1/orders", a.requireAuth(http.HandlerFunc(a.listOrders)))
	mux.Handle("GET /api/v1/orders/{id}", a.requireAuth(http.HandlerFunc(a.getOrder)))
	return a.logging(mux)
}

func (a *API) ready(w http.ResponseWriter,r *http.Request) {
	ctx,cancel := context.WithTimeout(r.Context(),time.Second)
	defer cancel()
	if err := a.repo.DB.Ping(ctx); err != nil { Error(w,http.StatusServiceUnavailable,"database unavailable"); return }
	if err := a.redis.Ping(ctx).Err(); err != nil { Error(w,http.StatusServiceUnavailable,"redis unavailable"); return }
	JSON(w,http.StatusOK,map[string]string{"status":"ready"})
}

func (a *API) register(w http.ResponseWriter,r *http.Request) {
	var in struct{Name,Email,Password string}
	if !decode(w,r,&in) { return }
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if strings.TrimSpace(in.Name)=="" || !strings.Contains(in.Email,"@") || len(in.Password)<8 {
		Error(w,http.StatusBadRequest,"name, valid email and password with at least 8 chars are required"); return
	}
	hash,err := auth.Hash(in.Password)
	if err != nil { a.internal(w,err); return }
	u,err := a.repo.CreateUser(r.Context(),strings.TrimSpace(in.Name),in.Email,hash)
	if err != nil { Error(w,http.StatusConflict,"email already registered"); return }
	token,err := a.auth.Generate(u.ID)
	if err != nil { a.internal(w,err); return }
	JSON(w,http.StatusCreated,map[string]any{"user":u,"token":token})
}

func (a *API) login(w http.ResponseWriter,r *http.Request) {
	var in struct{Email,Password string}
	if !decode(w,r,&in) { return }
	u,hash,err := a.repo.UserCredentialsByEmail(r.Context(),strings.ToLower(strings.TrimSpace(in.Email)))
	if err != nil || auth.Compare(hash,in.Password)!=nil { Error(w,http.StatusUnauthorized,"invalid credentials"); return }
	token,err := a.auth.Generate(u.ID)
	if err != nil { a.internal(w,err); return }
	JSON(w,http.StatusOK,map[string]any{"user":u,"token":token})
}

func (a *API) createProduct(w http.ResponseWriter,r *http.Request) {
	var in struct{Name,SKU string; PriceCents int64 `json:"price_cents"`; Stock int `json:"stock"`}
	if !decode(w,r,&in) { return }
	if strings.TrimSpace(in.Name)=="" || strings.TrimSpace(in.SKU)=="" || in.PriceCents<=0 || in.Stock<0 {
		Error(w,http.StatusBadRequest,"invalid product"); return
	}
	p,err := a.repo.CreateProduct(r.Context(),in.Name,strings.ToUpper(in.SKU),in.PriceCents,in.Stock)
	if err != nil { Error(w,http.StatusConflict,"sku already exists"); return }
	JSON(w,http.StatusCreated,p)
}

func (a *API) listProducts(w http.ResponseWriter,r *http.Request) {
	p,err := a.repo.ListProducts(r.Context())
	if err != nil { a.internal(w,err); return }
	JSON(w,http.StatusOK,p)
}

func (a *API) createOrder(w http.ResponseWriter,r *http.Request) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key=="" { Error(w,http.StatusBadRequest,"Idempotency-Key header is required"); return }
	userID := r.Context().Value(userKey).(string)
	cacheKey := "idem:"+userID+":"+key
	if existing,err := a.redis.Get(r.Context(),cacheKey).Result(); err==nil {
		o,err := a.repo.GetOrder(r.Context(),existing,userID)
		if err==nil { JSON(w,http.StatusOK,o); return }
	}
	ok,err := a.redis.SetNX(r.Context(),cacheKey,"processing",2*time.Minute).Result()
	if err != nil { a.internal(w,err); return }
	if !ok { Error(w,http.StatusConflict,"request with this idempotency key is already processing"); return }

	var in struct{Items []repository.NewOrderItem `json:"items"`}
	if !decode(w,r,&in) { a.redis.Del(r.Context(),cacheKey); return }
	if len(in.Items)==0 { a.redis.Del(r.Context(),cacheKey); Error(w,http.StatusBadRequest,"at least one item is required"); return }
	o,err := a.repo.CreateOrder(r.Context(),userID,in.Items)
	if err != nil { a.redis.Del(r.Context(),cacheKey); Error(w,http.StatusUnprocessableEntity,err.Error()); return }
	a.redis.Set(r.Context(),cacheKey,o.ID,24*time.Hour)
	JSON(w,http.StatusAccepted,o)
}

func (a *API) listOrders(w http.ResponseWriter,r *http.Request) {
	userID := r.Context().Value(userKey).(string)
	o,err := a.repo.ListOrders(r.Context(),userID)
	if err != nil { a.internal(w,err); return }
	JSON(w,http.StatusOK,o)
}

func (a *API) getOrder(w http.ResponseWriter,r *http.Request) {
	userID := r.Context().Value(userKey).(string)
	o,err := a.repo.GetOrder(r.Context(),r.PathValue("id"),userID)
	if errors.Is(err,repository.ErrNotFound) { Error(w,http.StatusNotFound,"order not found"); return }
	if err != nil { a.internal(w,err); return }
	JSON(w,http.StatusOK,o)
}

func (a *API) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h,"Bearer ") { Error(w,http.StatusUnauthorized,"missing bearer token"); return }
		id,err := a.auth.Parse(strings.TrimPrefix(h,"Bearer "))
		if err != nil { Error(w,http.StatusUnauthorized,"invalid token"); return }
		next.ServeHTTP(w,r.WithContext(context.WithValue(r.Context(),userKey,id)))
	})
}

func (a *API) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		start:=time.Now()
		next.ServeHTTP(w,r)
		a.log.Info("http_request",zap.String("method",r.Method),zap.String("path",r.URL.Path),zap.Duration("duration",time.Since(start)))
	})
}

func decode(w http.ResponseWriter,r *http.Request,v any) bool {
	dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20))
	dec.DisallowUnknownFields()
	if err:=dec.Decode(v); err!=nil { Error(w,http.StatusBadRequest,"invalid json body"); return false }
	return true
}

func JSON(w http.ResponseWriter,status int,v any) {
	w.Header().Set("Content-Type","application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter,status int,msg string) { JSON(w,status,map[string]string{"error":msg}) }

func (a *API) internal(w http.ResponseWriter,err error) {
	a.log.Error("internal_error",zap.Error(err))
	Error(w,http.StatusInternalServerError,"internal server error")
}
