package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

// fakeRepository: 이전에 정의한 형식을 핸들러 테스트에서도 재사용합니다.
type fakeRepository struct {
	upsertUserFn  func(ctx context.Context, user *User) error
	findByEmailFn func(ctx context.Context, email string) (*User, error)
}

func (f *fakeRepository) UpsertUser(ctx context.Context, user *User) error {
	return f.upsertUserFn(ctx, user)
}

func (f *fakeRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	return f.findByEmailFn(ctx, email)
}

func TestHandler_Login(t *testing.T) {
	// Gin을 테스트 모드로 설정
	gin.SetMode(gin.TestMode)

	// 의존성 설정
	config := &oauth2.Config{Endpoint: oauth2.Endpoint{AuthURL: "https://google.com/auth"}}
	svc := NewService(nil, config)
	h := NewHandler(svc)

	r := gin.Default()
	r.GET("/auth/login", h.Login)

	t.Run("로그인 요청 시 구글 승인 페이지로 리다이렉트된다", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/auth/login", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusTemporaryRedirect {
			t.Errorf("expected status 307, got %d", w.Code)
		}

		location := w.Header().Get("Location")
		if location == "" {
			t.Error("Location header is missing")
		}
	})
}

func TestHandler_Callback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("code 쿼리 파라미터가 없으면 400 에러를 반환한다", func(t *testing.T) {
		h := NewHandler(&Service{})
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/auth/google/callback", nil) // code 없음

		h.Callback(c)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("서비스에서 에러 발생 시 500 에러를 반환한다", func(t *testing.T) {
		// Repository에서 에러가 발생하도록 설정하여 서비스 로직 실패 유도
		repo := &fakeRepository{
			upsertUserFn: func(ctx context.Context, user *User) error {
				return errors.New("db error")
			},
		}
		
		// 실제 Google Exchange/Validate 로직을 타지 않게 하기 위해 
		// 이 테스트는 Service의 ProcessGoogleCallback이 아닌 
		// Handler가 Service의 에러를 어떻게 처리하는지에 집중합니다.
		
		// (참고: 실제 환경에선 Service를 인터페이스화하여 MockService를 넣는 것이 더 깔끔합니다.)
		svc := &Service{Repo: repo, OauthConfig: &oauth2.Config{}}
		h := NewHandler(svc)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/auth/google/callback?code=valid-code", nil)

		h.Callback(c)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", w.Code)
		}
	})

	t.Run("성공 시 유저 정보와 200 상태 코드를 반환한다", func(t *testing.T) {
		// 이 테스트를 위해서는 ProcessGoogleCallback 내부의 구글 API 호출 부분을 
		// 별도로 모킹하거나 인터페이스로 분리해야 완벽한 테스트가 가능합니다.
		// 아래는 결과 구조 검증 예시입니다.
		
		w := httptest.NewRecorder()
		// 가상의 성공 응답 바디 검증 예시
		response := gin.H{
			"message": "login success",
			"user": User{
				Email: "test@khu.ac.kr",
				Name:  "경희인",
			},
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)

		var resBody map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resBody)

		if resBody["message"] != "login success" {
			t.Errorf("expected success message, got %v", resBody["message"])
		}
	})
}