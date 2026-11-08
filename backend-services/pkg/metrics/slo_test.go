package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestClassify(t *testing.T) {
	cases := map[int]Outcome{
		0:   OutcomeBad,
		200: OutcomeGood,
		201: OutcomeGood,
		302: OutcomeGood,
		400: OutcomeNeutral,
		401: OutcomeNeutral,
		404: OutcomeNeutral,
		409: OutcomeNeutral,
		500: OutcomeBad,
		502: OutcomeBad,
		503: OutcomeBad,
	}
	for status, want := range cases {
		if got := Classify(status); got != want {
			t.Errorf("Classify(%d) = %q, want %q", status, got, want)
		}
	}
}

func TestTrackCountsRequestClasses(t *testing.T) {
	handler := func(status int) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		})
	}

	// Неверный пароль на auth: 400 → neutral.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	Track("auth", handler(http.StatusBadRequest)).ServeHTTP(httptest.NewRecorder(), req)
	// 502 на слое feed.
	req = httptest.NewRequest(http.MethodGet, "/api/feed/", nil)
	Track("feed", handler(http.StatusBadGateway)).ServeHTTP(httptest.NewRecorder(), req)

	got := testutil.ToFloat64(SLORequests.WithLabelValues("auth", "neutral", "bad_request"))
	if got != 1 {
		t.Errorf("auth neutral = %v, want 1", got)
	}
	got = testutil.ToFloat64(SLORequests.WithLabelValues("feed", "bad", "bad_gateway"))
	if got != 1 {
		t.Errorf("feed bad = %v, want 1", got)
	}
}

// Заголовок X-SLO-Service не должен влиять на слой: иначе запрос,
// прошедший через gateway и сервис, посчитался бы дважды.
func TestTrackIgnoresGatewayHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/feed/", nil)
	req.Header.Set("X-SLO-Service", "feed")

	Track("feed", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).
		ServeHTTP(httptest.NewRecorder(), req)

	got := testutil.ToFloat64(SLORequests.WithLabelValues("feed", "good", "ok"))
	if got != 1 {
		t.Errorf("feed good = %v, want ровно 1 (без двойного учёта)", got)
	}
}

func TestTrackSkipsHiddenRoutes(t *testing.T) {
	// Пустое имя слоя — служебный маршрут (/health), в SLI не попадает.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	Track("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).
		ServeHTTP(httptest.NewRecorder(), req)
}
