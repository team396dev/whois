# WHOIS Bulk Lookup Service — Implementation Plan

## Цель

Веб-сервис для массового WHOIS-пробива доменов. На входе — список доменов,
на выходе — таблица «домен → регистратор» и статистика по регистраторам.
Backend на Go, параллельная обработка в `runtime.NumCPU()` воркеров, фронт — чистый HTML/CSS/JS.

---

## 1. Анализ зон из примера и WHOIS-серверы

| TLD  | WHOIS-сервер                    | Поле регистратора    | Особенности |
|------|---------------------------------|----------------------|-------------|
| .nl  | whois.domain-registry.nl        | `Registrar:`         | Стандартный, стабильный |
| .fi  | whois.fi                        | `registrar:`         | Стандартный |
| .dk  | whois.dk-hostmaster.dk          | `Registrar:`         | Handle-система, парсинг нестандартный |
| .cz  | whois.nic.cz                    | `registrar:`         | Чистый формат |
| .es  | whois.nic.es                    | `Registrar Name:`    | Бывают ограничения по rate-limit |
| .pl  | whois.dns.pl                    | `REGISTRAR:`         | Стандартный |
| .at  | whois.nic.at                    | `registrar:`         | Стандартный |
| .it  | whois.nic.it                    | `Registrar`          | Стандартный |
| .com | whois.verisign-grs.com          | `Registrar:`         | |
| .net | whois.verisign-grs.com          | `Registrar:`         | |
| .org | whois.pir.org                   | `Registrar:`         | |
| .io  | whois.nic.io                    | `Registrar:`         | |
| .de  | whois.denic.de                  | `Registrar:`         | |
| .uk  | whois.nic.uk                    | `Registrar:`         | |
| .ru  | whois.tcinet.ru                 | `registrar:`         | |
| .eu  | whois.eu                        | `Registrar:`         | |

Список будет расширяться — нужна таблица `map[string]TLDConfig` в коде.

### Проблемные зоны и решение

**Проблема:** Некоторые ccTLD ограничивают WHOIS или прячут регистратора:
- `.es` — WHOIS работает, но иногда throttling
- `.dk` — нестандартные поля

**Решение — двухуровневый фоллбэк:**
1. **Уровень 1 — классический WHOIS (port 43)** — основной путь
2. **Уровень 2 — RDAP** — если WHOIS вернул ошибку или пустой регистратор.
   RDAP — официальный преемник WHOIS (RFC 7483), реестры обязаны его поддерживать.
   Запрос к IANA bootstrap: `https://rdap.iana.org/domain/<domain>` → редирект на реестр → JSON.
   Парсим `entities[].roles == "registrar"` → `vcardArray[1][]["fn"]`.
   RDAP — это прямой запрос к реестру, не сторонний сервис.

---

## 2. Структура проекта

```
whois-parser/
├── cmd/
│   └── server/
│       └── main.go              # точка входа, HTTP-сервер
├── internal/
│   ├── whois/
│   │   ├── client.go            # TCP-клиент port 43
│   │   ├── servers.go           # map TLD → TLDConfig (сервер + regex поля)
│   │   └── parser.go            # универсальный + per-TLD парсеры
│   ├── rdap/
│   │   └── client.go            # RDAP fallback (HTTP + JSON)
│   ├── lookup/
│   │   ├── service.go           # оркестрация: WHOIS → RDAP fallback
│   │   └── worker.go            # воркер-пул runtime.NumCPU()
│   └── api/
│       ├── handler.go           # HTTP-хэндлеры
│       └── sse.go               # Server-Sent Events стриминг результатов
├── web/
│   ├── index.html               # SPA (встраивается в бинарь через embed)
│   ├── style.css
│   └── app.js
├── go.mod
└── go.sum
```

---

## 3. Backend — ключевые компоненты

### 3.1 TLD Config (`internal/whois/servers.go`)

```go
type TLDConfig struct {
    Server         string   // whois.nic.example:43
    RegistrarField []string // ["Registrar:", "registrar:"] — пробуем по порядку
    QueryPrefix    string   // некоторые серверы требуют "-domain " перед именем
    RateLimit      time.Duration // задержка между запросами к этому серверу
}

var tldConfigs = map[string]TLDConfig{
    "nl": {Server: "whois.domain-registry.nl:43", RegistrarField: []string{"Registrar:"}},
    "fi": {Server: "whois.fi:43",                 RegistrarField: []string{"registrar:"}},
    "dk": {Server: "whois.dk-hostmaster.dk:43",   RegistrarField: []string{"Registrar:"}},
    "cz": {Server: "whois.nic.cz:43",             RegistrarField: []string{"registrar:"}},
    "es": {Server: "whois.nic.es:43",             RegistrarField: []string{"Registrar Name:", "Registrar:"}},
    "pl": {Server: "whois.dns.pl:43",             RegistrarField: []string{"REGISTRAR:", "registrar:"}},
    "at": {Server: "whois.nic.at:43",             RegistrarField: []string{"registrar:"}},
    "it": {Server: "whois.nic.it:43",             RegistrarField: []string{"Registrar"}},
    // ... далее расширяется
}
```

Если TLD не найден в таблице — fallback на IANA WHOIS (`whois.iana.org`),
который вернёт `refer:` с адресом нужного сервера (двухступенчатый WHOIS).

### 3.2 WHOIS Client (`internal/whois/client.go`)

```go
func Query(domain string, cfg TLDConfig) (string, error) {
    conn, err := net.DialTimeout("tcp", cfg.Server, 10*time.Second)
    // send: domain + "\r\n"
    // read response (deadline 15s)
    // return raw text
}
```

### 3.3 Parser (`internal/whois/parser.go`)

```go
func ExtractRegistrar(rawWhois string, fields []string) string {
    // построчный скан, ищем первый match по полю из fields
    // возвращаем значение после ":"
    // нормализация: trim, убираем URL, оставляем имя
}
```

### 3.4 RDAP Fallback (`internal/rdap/client.go`)

```go
func QueryRDAP(domain string) (string, error) {
    // GET https://rdap.iana.org/domain/<domain>  (follow redirect к реестру)
    // Parse JSON: .entities[] where .roles contains "registrar"
    // Extract .vcardArray[1] where [0] == "fn" → return [3]
}
```

### 3.5 Lookup Service (`internal/lookup/service.go`)

```go
type Result struct {
    Domain    string
    Registrar string
    Source    string // "whois" | "rdap" | "error"
    Error     string
}

func Lookup(domain string) Result {
    tld := extractTLD(domain)
    cfg, ok := tldConfigs[tld]
    
    if ok {
        raw, err := whois.Query(domain, cfg)
        if err == nil {
            reg := whois.ExtractRegistrar(raw, cfg.RegistrarField)
            if reg != "" {
                return Result{Domain: domain, Registrar: reg, Source: "whois"}
            }
        }
    }
    
    // RDAP fallback
    reg, err := rdap.QueryRDAP(domain)
    if err == nil && reg != "" {
        return Result{Domain: domain, Registrar: reg, Source: "rdap"}
    }
    
    return Result{Domain: domain, Error: "not found"}
}
```

### 3.6 Worker Pool (`internal/lookup/worker.go`)

```go
func RunBatch(domains []string, resultCh chan<- Result) {
    numWorkers := runtime.NumCPU()
    jobs := make(chan string, len(domains))
    
    var wg sync.WaitGroup
    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for domain := range jobs {
                resultCh <- Lookup(domain)
            }
        }()
    }
    
    for _, d := range domains {
        jobs <- d
    }
    close(jobs)
    
    wg.Wait()
    close(resultCh)
}
```

### 3.7 API (`internal/api/handler.go`)

**POST /api/lookup** — запускает пробив, возвращает `text/event-stream` (SSE).
Стриминг: каждый результат пушится клиенту сразу по мере готовности.

```
POST /api/lookup
Content-Type: application/json
{ "domains": ["example.nl", "test.fi", ...] }

→ 200 text/event-stream
data: {"domain":"example.nl","registrar":"TransIP B.V.","source":"whois"}\n\n
data: {"domain":"test.fi","registrar":"Ficora","source":"whois"}\n\n
...
data: {"type":"done","stats":{"TransIP B.V.":5,"Namecheap":3}}\n\n
```

SSE даёт real-time обновление UI без polling.

---

## 4. Frontend (`web/`)

### 4.1 Layout

```
┌─────────────────────────────────────────────────────────┐
│  WHOIS Bulk Lookup                                       │
├──────────────────────┬──────────────────────────────────┤
│                      │  Domain              Registrar   │
│  [textarea]          │  ─────────────────────────────  │
│  one domain per line │  slotswolf.nl        TransIP     │
│                      │  slotwolf.fi         Traficom    │
│  [  Lookup  ]        │  betvili.dk          Ascio       │
│                      │  ...                 ...         │
│  ■ 36 domains        │                                  │
│                      ├──────────────────────────────────┤
│                      │  Registrar Stats                 │
│                      │  TransIP B.V.   ████░░  42%      │
│                      │  Namecheap      ███░░░  31%      │
│                      │  GoDaddy        ██░░░░  17%      │
│                      │  Other           █░░░░  10%      │
└──────────────────────┴──────────────────────────────────┘
```

### 4.2 Технологии фронта

- Чистый HTML5 + CSS3 + Vanilla JS (без фреймворков, встраивается в бинарь)
- CSS: CSS variables, Inter/system font-stack, тёмная тема
- JS: EventSource для SSE, живое обновление таблицы
- Stat bars: чистый CSS (ширина через inline style)

### 4.3 UX детали

- Строки таблицы появляются по мере ответов (SSE)
- Source "rdap" = маленький badge серого цвета рядом с регистратором
- Source "error" = красный текст "lookup failed"
- Кнопка "Copy CSV" копирует результаты в буфер
- Счётчик прогресса: "23 / 36"
- После окончания — кнопка "New Lookup" очищает форму

---

## 5. Embed и сборка

```go
//go:embed web
var webFS embed.FS

// в main.go:
http.Handle("/", http.FileServer(http.FS(webFS)))
```

Итоговый бинарь — один файл, без внешних зависимостей.

---

## 6. Rate Limiting и надёжность

- Per-server rate limit через `golang.org/x/time/rate` (лимитер на каждый WHOIS-сервер)
- Timeout per запрос: 15 секунд
- При таймауте/ошибке — автоматический retry 1 раз, затем RDAP fallback
- Максимум доменов за один запрос: 500 (настраивается через env `MAX_DOMAINS`)
- Семафор на количество одновременных запросов к одному серверу: 3

---

## 7. Порядок реализации

| Шаг | Что делаем | Файлы |
|-----|-----------|-------|
| 1 | `go mod init`, структура директорий | go.mod |
| 2 | TLD server map + WHOIS TCP client | internal/whois/ |
| 3 | Универсальный парсер регистратора | internal/whois/parser.go |
| 4 | RDAP fallback клиент | internal/rdap/client.go |
| 5 | Lookup service (WHOIS → RDAP) | internal/lookup/service.go |
| 6 | Worker pool (NumCPU воркеров) | internal/lookup/worker.go |
| 7 | HTTP + SSE хэндлеры | internal/api/ |
| 8 | Статика: HTML/CSS layout | web/ |
| 9 | JS SSE клиент + живая таблица | web/app.js |
| 10 | Stats: подсчёт + рендер баров | web/app.js |
| 11 | embed + main.go сборка | cmd/server/main.go |
| 12 | Тест на примере 36 доменов из ТЗ | — |

---

## 8. Зависимости Go

```
// go.mod - только стандартная библиотека + одна утилита для rate-limit
golang.org/x/time v0.x  // rate.Limiter
```

Всё остальное — `net`, `encoding/json`, `net/http`, `embed`, `runtime`, `sync` — стандартная библиотека.

---

## 9. Риски и митигация

| Риск | Вероятность | Митигация |
|------|------------|-----------|
| WHOIS-сервер блокирует по IP при массовом пробиве | Средняя | Rate limit по 1-2 req/s на сервер, retry с backoff |
| .es WHOIS возвращает пустой регистратор | Средняя | RDAP fallback (nic.es поддерживает RDAP) |
| .dk нестандартный формат ответа | Средняя | Отдельный per-TLD парсер для dk |
| Неизвестный TLD не в таблице | Высокая | IANA WHOIS двухшаговый lookup → автодискавери сервера |
| RDAP redirect loop / timeout | Низкая | Timeout 10s + follow max 5 redirects |

---

## 10. Конфигурация (env vars)

```
PORT=8080          # порт сервера
MAX_DOMAINS=500    # лимит доменов на запрос
WHOIS_TIMEOUT=15   # секунды на один WHOIS запрос
RDAP_TIMEOUT=10    # секунды на один RDAP запрос
WORKERS=0          # 0 = runtime.NumCPU()
```
