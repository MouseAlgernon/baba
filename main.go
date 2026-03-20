package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/big"
	"net/http"
	"os"
	"strings"
)

// ============================================================
// НАСТРОЙКИ
// ============================================================

const PAGE_SIZE = 500

// ============================================================
// Алфавиты
// ============================================================

var (
	PAGE_ALPHABET []rune
	SEED_ALPHABET []rune
	pageIndex     map[rune]int
	seedIndex     map[rune]int
	MAX_SEED_LEN  int
	pageSpaceSize *big.Int
)

func init() {
	PAGE_ALPHABET = buildPageAlphabet()
	SEED_ALPHABET = buildSeedAlphabet()
	pageIndex = makeRuneIndex(PAGE_ALPHABET)
	seedIndex = makeRuneIndex(SEED_ALPHABET)

	pageBase := big.NewInt(int64(len(PAGE_ALPHABET)))
	pageSpaceSize = new(big.Int).Exp(pageBase, big.NewInt(int64(PAGE_SIZE)), nil)

	seedBase := float64(len(SEED_ALPHABET))
	pageSpace := float64(PAGE_SIZE) * math.Log(float64(len(PAGE_ALPHABET)))
	MAX_SEED_LEN = int(pageSpace / math.Log(seedBase))

	sb := big.NewInt(int64(len(SEED_ALPHABET)))
	for {
		limit := new(big.Int).Exp(sb, big.NewInt(int64(MAX_SEED_LEN+1)), nil)
		if limit.Cmp(pageSpaceSize) >= 0 {
			break
		}
		MAX_SEED_LEN++
	}

	log.Printf("Babylon Library ready: page=%d chars, seed max=%d chars",
		len(PAGE_ALPHABET), MAX_SEED_LEN)
}

func buildPageAlphabet() []rune {
	var chars []rune
	for r := rune(0x0430); r <= 0x044F; r++ { chars = append(chars, r) }
	for r := rune(0x0410); r <= 0x042F; r++ { chars = append(chars, r) }
	chars = append(chars, 'ё', 'Ё')
	chars = append(chars, []rune{
		' ', '\n', '.', ',', '!', '?', ';', ':', '-',
		'–', '—', '(', ')', '«', '»', '"', '\'', '…', '/', '\\',
	}...)
	return chars
}

func buildSeedAlphabet() []rune {
	chars := make([]rune, 0, 95)
	for i := 32; i <= 126; i++ {
		chars = append(chars, rune(i))
	}
	return chars
}

func makeRuneIndex(alphabet []rune) map[rune]int {
	m := make(map[rune]int, len(alphabet))
	for i, r := range alphabet {
		m[r] = i
	}
	return m
}

// ============================================================
// Биективная арифметика
// ============================================================

func seedToNumber(seed string) (*big.Int, error) {
	runes := []rune(seed)
	if len(runes) > MAX_SEED_LEN {
		return nil, fmt.Errorf("сид слишком длинный: %d символов, максимум %d", len(runes), MAX_SEED_LEN)
	}
	base := big.NewInt(int64(len(SEED_ALPHABET)))
	result := big.NewInt(0)
	for _, r := range runes {
		idx, ok := seedIndex[r]
		if !ok {
			return nil, fmt.Errorf("символ %q не входит в алфавит сида (ASCII 32–126)", r)
		}
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(idx)))
	}
	if result.Cmp(pageSpaceSize) >= 0 {
		return nil, fmt.Errorf("сид выходит за пределы пространства страниц")
	}
	return result, nil
}

func numberToSeed(n *big.Int) string {
	if n.Sign() == 0 {
		return string(SEED_ALPHABET[0])
	}
	base := big.NewInt(int64(len(SEED_ALPHABET)))
	tmp := new(big.Int).Set(n)
	mod := new(big.Int)
	var chars []rune
	for tmp.Sign() > 0 {
		tmp.DivMod(tmp, base, mod)
		chars = append(chars, SEED_ALPHABET[int(mod.Int64())])
	}
	for i, j := 0, len(chars)-1; i < j; i, j = i+1, j-1 {
		chars[i], chars[j] = chars[j], chars[i]
	}
	return string(chars)
}

func pageToNumber(page []rune) (*big.Int, error) {
	base := big.NewInt(int64(len(PAGE_ALPHABET)))
	result := big.NewInt(0)
	for _, r := range page {
		idx, ok := pageIndex[r]
		if !ok {
			return nil, fmt.Errorf("символ %q не входит в алфавит страницы", r)
		}
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(idx)))
	}
	return result, nil
}

func numberToPage(n *big.Int) []rune {
	base := big.NewInt(int64(len(PAGE_ALPHABET)))
	page := make([]rune, PAGE_SIZE)
	tmp := new(big.Int).Set(n)
	mod := new(big.Int)
	for i := PAGE_SIZE - 1; i >= 0; i-- {
		tmp.DivMod(tmp, base, mod)
		page[i] = PAGE_ALPHABET[int(mod.Int64())]
	}
	return page
}

func generateRandomSeed() (string, error) {
	max := big.NewInt(int64(len(SEED_ALPHABET)))
	chars := make([]rune, MAX_SEED_LEN)
	for i := range chars {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		chars[i] = SEED_ALPHABET[n.Int64()]
	}
	return string(chars), nil
}

// ============================================================
// HTTP API
// ============================================================

type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Page  string `json:"page,omitempty"`
	Seed  string `json:"seed,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// POST /api/generate  { "seed": "..." }
func handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, Response{Error: "метод не разрешён"})
		return
	}
	var req struct {
		Seed string `json:"seed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, Response{Error: "неверный JSON"})
		return
	}
	seed := strings.TrimSpace(req.Seed)
	if seed == "" {
		writeJSON(w, 400, Response{Error: "сид не может быть пустым"})
		return
	}
	n, err := seedToNumber(seed)
	if err != nil {
		writeJSON(w, 400, Response{Error: err.Error()})
		return
	}
	page := numberToPage(n)
	writeJSON(w, 200, Response{OK: true, Page: string(page), Seed: seed})
}

// POST /api/recover  { "page": "..." }
func handleRecover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, Response{Error: "метод не разрешён"})
		return
	}
	var req struct {
		Page string `json:"page"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, Response{Error: "неверный JSON"})
		return
	}
	runes := []rune(req.Page)
	if len(runes) != PAGE_SIZE {
		writeJSON(w, 400, Response{
			Error: fmt.Sprintf("страница должна содержать ровно %d символов, получено %d", PAGE_SIZE, len(runes)),
		})
		return
	}
	n, err := pageToNumber(runes)
	if err != nil {
		writeJSON(w, 400, Response{Error: err.Error()})
		return
	}
	seed := numberToSeed(n)
	writeJSON(w, 200, Response{OK: true, Seed: seed, Page: req.Page})
}

// GET /api/random
func handleRandom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, Response{Error: "метод не разрешён"})
		return
	}
	seed, err := generateRandomSeed()
	if err != nil {
		writeJSON(w, 500, Response{Error: "ошибка генерации"})
		return
	}
	n, err := seedToNumber(seed)
	if err != nil {
		writeJSON(w, 500, Response{Error: err.Error()})
		return
	}
	page := numberToPage(n)
	writeJSON(w, 200, Response{OK: true, Seed: seed, Page: string(page)})
}

// GET /api/info
func handleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]any{
		"page_size":       PAGE_SIZE,
		"page_alpha_size": len(PAGE_ALPHABET),
		"seed_alpha_size": len(SEED_ALPHABET),
		"max_seed_len":    MAX_SEED_LEN,
	})
}

// ============================================================
// Static frontend (встроен в бинарник как строка)
// ============================================================

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(indexHTML))
}

// ============================================================
// main
// ============================================================

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/generate", handleGenerate)
	mux.HandleFunc("/api/recover", handleRecover)
	mux.HandleFunc("/api/random", handleRandom)
	mux.HandleFunc("/api/info", handleInfo)
	mux.HandleFunc("/", handleIndex)

	log.Printf("Listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

// ============================================================
// Встроенный HTML (один файл — удобно для деплоя)
// ============================================================

const indexHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Вавилонская Библиотека</title>
<style>
  :root {
    --bg:       #0d0b08;
    --surface:  #141209;
    --border:   #2a2418;
    --gold:     #c9a84c;
    --gold-dim: #7a6128;
    --text:     #d4c9a8;
    --muted:    #6b5f42;
    --red:      #c0392b;
    --green:    #27ae60;
    --font-mono: 'Courier New', Courier, monospace;
    --font-serif: Georgia, 'Times New Roman', serif;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  html, body { height: 100%; }

  body {
    background: var(--bg);
    color: var(--text);
    font-family: var(--font-serif);
    display: flex;
    flex-direction: column;
    min-height: 100vh;
  }

  /* ── Шапка ── */
  header {
    border-bottom: 1px solid var(--border);
    padding: 2rem 2rem 1.5rem;
    text-align: center;
  }
  header h1 {
    font-size: clamp(1.4rem, 4vw, 2.2rem);
    color: var(--gold);
    letter-spacing: .12em;
    text-transform: uppercase;
  }
  header p {
    margin-top: .5rem;
    color: var(--muted);
    font-size: .85rem;
    letter-spacing: .05em;
  }

  /* ── Вкладки ── */
  nav {
    display: flex;
    border-bottom: 1px solid var(--border);
  }
  nav button {
    flex: 1;
    padding: .9rem .5rem;
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    color: var(--muted);
    font-family: var(--font-serif);
    font-size: .9rem;
    letter-spacing: .06em;
    cursor: pointer;
    transition: color .2s, border-color .2s;
  }
  nav button:hover { color: var(--text); }
  nav button.active {
    color: var(--gold);
    border-bottom-color: var(--gold);
  }

  /* ── Основной контент ── */
  main {
    flex: 1;
    max-width: 860px;
    width: 100%;
    margin: 0 auto;
    padding: 2rem 1.5rem;
  }

  .panel { display: none; }
  .panel.active { display: block; }

  label {
    display: block;
    color: var(--gold-dim);
    font-size: .75rem;
    letter-spacing: .1em;
    text-transform: uppercase;
    margin-bottom: .4rem;
  }

  input[type=text], textarea {
    width: 100%;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 3px;
    color: var(--text);
    font-family: var(--font-mono);
    font-size: .82rem;
    padding: .7rem .9rem;
    outline: none;
    resize: vertical;
    transition: border-color .2s;
  }
  input[type=text]:focus, textarea:focus {
    border-color: var(--gold-dim);
  }

  .char-count {
    text-align: right;
    font-size: .72rem;
    color: var(--muted);
    margin-top: .25rem;
  }
  .char-count.over { color: var(--red); }

  .btn {
    display: inline-flex;
    align-items: center;
    gap: .5rem;
    margin-top: 1rem;
    padding: .65rem 1.5rem;
    background: none;
    border: 1px solid var(--gold-dim);
    border-radius: 3px;
    color: var(--gold);
    font-family: var(--font-serif);
    font-size: .9rem;
    letter-spacing: .06em;
    cursor: pointer;
    transition: background .2s, border-color .2s;
  }
  .btn:hover { background: rgba(201,168,76,.08); border-color: var(--gold); }
  .btn:active { background: rgba(201,168,76,.15); }
  .btn:disabled { opacity: .4; cursor: not-allowed; }
  .btn-row { display: flex; gap: .75rem; flex-wrap: wrap; }

  /* ── Результат ── */
  .result {
    margin-top: 1.75rem;
    border: 1px solid var(--border);
    border-radius: 3px;
    overflow: hidden;
  }
  .result-header {
    background: var(--surface);
    padding: .55rem 1rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid var(--border);
  }
  .result-header span {
    font-size: .72rem;
    letter-spacing: .1em;
    text-transform: uppercase;
    color: var(--gold-dim);
  }
  .copy-btn {
    background: none;
    border: 1px solid var(--border);
    border-radius: 2px;
    color: var(--muted);
    font-size: .72rem;
    padding: .2rem .6rem;
    cursor: pointer;
    transition: color .2s, border-color .2s;
  }
  .copy-btn:hover { color: var(--gold); border-color: var(--gold-dim); }

  .result-body {
    padding: 1rem;
    font-family: var(--font-mono);
    font-size: .82rem;
    line-height: 1.7;
    white-space: pre-wrap;
    word-break: break-all;
    max-height: 360px;
    overflow-y: auto;
  }

  .status {
    margin-top: 1rem;
    padding: .5rem .9rem;
    border-radius: 3px;
    font-size: .82rem;
    display: none;
  }
  .status.ok  { background: rgba(39,174,96,.12);  border: 1px solid rgba(39,174,96,.3);  color: var(--green); display: block; }
  .status.err { background: rgba(192,57,43,.12);  border: 1px solid rgba(192,57,43,.3);  color: var(--red);   display: block; }

  /* ── Ссылка ── */
  .share-row {
    margin-top: .75rem;
    display: flex;
    gap: .5rem;
    align-items: center;
  }
  .share-row input {
    flex: 1;
    font-size: .75rem;
    padding: .4rem .7rem;
  }

  /* ── Инфо ── */
  .info-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 1rem;
    margin-top: 1rem;
  }
  .info-card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 3px;
    padding: 1rem;
    text-align: center;
  }
  .info-card .val {
    font-size: 1.6rem;
    color: var(--gold);
    font-family: var(--font-mono);
  }
  .info-card .lbl {
    margin-top: .3rem;
    font-size: .72rem;
    color: var(--muted);
    letter-spacing: .06em;
    text-transform: uppercase;
  }

  /* ── Спиннер ── */
  .spinner {
    display: inline-block;
    width: 1em; height: 1em;
    border: 2px solid var(--gold-dim);
    border-top-color: var(--gold);
    border-radius: 50%;
    animation: spin .6s linear infinite;
    vertical-align: middle;
  }
  @keyframes spin { to { transform: rotate(360deg); } }

  footer {
    border-top: 1px solid var(--border);
    padding: 1rem;
    text-align: center;
    color: var(--muted);
    font-size: .75rem;
    letter-spacing: .06em;
  }

  @media (max-width: 500px) {
    main { padding: 1.2rem 1rem; }
    nav button { font-size: .78rem; padding: .7rem .3rem; }
  }
</style>
</head>
<body>

<header>
  <h1>⟁ Вавилонская Библиотека</h1>
  <p>Каждая возможная страница существует. Найди свою.</p>
</header>

<nav>
  <button class="active" onclick="switchTab('generate')">Генерировать</button>
  <button onclick="switchTab('recover')">Восстановить</button>
  <button onclick="switchTab('random')">Случайная</button>
  <button onclick="switchTab('about')">О библиотеке</button>
</nav>

<main>

  <!-- ── ГЕНЕРИРОВАТЬ ── -->
  <div class="panel active" id="tab-generate">
    <label for="seed-input">Сид (адрес страницы) — до <span id="max-len">489</span> символов ASCII</label>
    <input type="text" id="seed-input" placeholder="Введите любой текст на ASCII..." maxlength="489"
           oninput="updateSeedCount()" onkeydown="if(event.key==='Enter')doGenerate()">
    <div class="char-count" id="seed-count">0 / 489</div>

    <div class="btn-row">
      <button class="btn" onclick="doGenerate()" id="btn-gen">Открыть страницу</button>
    </div>

    <div class="status" id="gen-status"></div>

    <div class="result" id="gen-result" style="display:none">
      <div class="result-header">
        <span>Страница</span>
        <button class="copy-btn" onclick="copyText('gen-page')">Копировать</button>
      </div>
      <div class="result-body" id="gen-page"></div>
    </div>

    <div class="share-row" id="gen-share" style="display:none">
      <input type="text" id="gen-share-url" readonly onclick="this.select()">
      <button class="copy-btn" onclick="copyField('gen-share-url')">Скопировать ссылку</button>
    </div>
  </div>

  <!-- ── ВОССТАНОВИТЬ ── -->
  <div class="panel" id="tab-recover">
    <label for="page-input">Вставьте страницу (ровно 500 символов)</label>
    <textarea id="page-input" rows="10"
      placeholder="Вставьте текст страницы сюда..."
      oninput="updatePageCount()"></textarea>
    <div class="char-count" id="page-count">0 / 500</div>

    <div class="btn-row">
      <button class="btn" onclick="doRecover()" id="btn-rec">Найти адрес</button>
    </div>

    <div class="status" id="rec-status"></div>

    <div class="result" id="rec-result" style="display:none">
      <div class="result-header">
        <span>Сид (адрес)</span>
        <button class="copy-btn" onclick="copyText('rec-seed')">Копировать</button>
      </div>
      <div class="result-body" id="rec-seed"></div>
    </div>
  </div>

  <!-- ── СЛУЧАЙНАЯ ── -->
  <div class="panel" id="tab-random">
    <p style="color:var(--muted);font-size:.9rem;line-height:1.7;margin-bottom:1.2rem">
      Случайная страница выбирается из 86<sup>500</sup> возможных — числа,
      которое несравнимо больше количества атомов во вселенной.
      Вероятность получить одну и ту же страницу дважды практически равна нулю.
    </p>

    <div class="btn-row">
      <button class="btn" onclick="doRandom()" id="btn-rnd">Открыть случайную страницу</button>
    </div>

    <div class="status" id="rnd-status"></div>

    <div class="result" id="rnd-seed-result" style="display:none">
      <div class="result-header">
        <span>Адрес (сид)</span>
        <button class="copy-btn" onclick="copyText('rnd-seed')">Копировать адрес</button>
      </div>
      <div class="result-body" id="rnd-seed"></div>
    </div>

    <div class="result" id="rnd-page-result" style="display:none">
      <div class="result-header">
        <span>Страница</span>
        <button class="copy-btn" onclick="copyText('rnd-page')">Копировать страницу</button>
      </div>
      <div class="result-body" id="rnd-page"></div>
    </div>

    <div class="share-row" id="rnd-share" style="display:none">
      <input type="text" id="rnd-share-url" readonly onclick="this.select()">
      <button class="copy-btn" onclick="copyField('rnd-share-url')">Скопировать ссылку</button>
    </div>
  </div>

  <!-- ── О БИБЛИОТЕКЕ ── -->
  <div class="panel" id="tab-about">
    <div class="info-grid" id="info-grid">
      <div class="info-card"><div class="val">500</div><div class="lbl">Символов на странице</div></div>
      <div class="info-card"><div class="val">86</div><div class="lbl">Символов в алфавите</div></div>
      <div class="info-card"><div class="val">95</div><div class="lbl">Символов в алфавите сида</div></div>
      <div class="info-card"><div class="val" id="info-maxseed">489</div><div class="lbl">Макс. длина сида</div></div>
    </div>

    <div style="margin-top:1.75rem;line-height:1.8;color:var(--text);font-size:.92rem">
      <p style="margin-bottom:1rem">
        <strong style="color:var(--gold)">Вавилонская библиотека</strong> — концепция Хорхе Луиса Борхеса:
        библиотека, содержащая все возможные книги. Эта реализация охватывает все возможные
        страницы из 500 символов русского алфавита, знаков препинания и переноса строки.
      </p>
      <p style="margin-bottom:1rem">
        <strong style="color:var(--gold)">Как это работает:</strong> страница — это число
        в системе счисления с основанием 86 (размер алфавита). Сид — то же число,
        но в системе счисления с основанием 95 (ASCII 32–126). Преобразование строго
        обратимо: каждому сиду соответствует ровно одна страница, и наоборот.
      </p>
      <p>
        <strong style="color:var(--gold)">Никаких хешей.</strong> Никакой базы данных.
        Страницы не хранятся — они вычисляются мгновенно из адреса (сида),
        а адрес восстанавливается из страницы. Любая страница, которую ты введёшь,
        имеет свой уникальный адрес в этой библиотеке — даже если никто никогда
        её не видел.
      </p>
    </div>
  </div>

</main>

<footer>Вавилонская Библиотека · 86<sup>500</sup> страниц · каждая уникальна</footer>

<script>
const MAX_SEED = 489;

// ── Вкладки ──
function switchTab(name) {
  document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
  document.querySelectorAll('nav button').forEach(b => b.classList.remove('active'));
  document.getElementById('tab-' + name).classList.add('active');
  event.currentTarget.classList.add('active');
}

// ── Счётчики ──
function updateSeedCount() {
  const el = document.getElementById('seed-input');
  const cnt = document.getElementById('seed-count');
  const n = [...el.value].length;
  cnt.textContent = n + ' / ' + MAX_SEED;
  cnt.classList.toggle('over', n > MAX_SEED);
}
function updatePageCount() {
  const el = document.getElementById('page-input');
  const cnt = document.getElementById('page-count');
  const n = [...el.value].length;
  cnt.textContent = n + ' / 500';
  cnt.classList.toggle('over', n !== 500);
}

// ── Статус ──
function setStatus(id, msg, isErr) {
  const el = document.getElementById(id);
  el.textContent = msg;
  el.className = 'status ' + (isErr ? 'err' : 'ok');
}
function clearStatus(id) {
  document.getElementById(id).className = 'status';
}

// ── Копирование ──
function copyText(id) {
  const text = document.getElementById(id).textContent;
  navigator.clipboard.writeText(text).then(() => flash(id));
}
function copyField(id) {
  const el = document.getElementById(id);
  el.select();
  navigator.clipboard.writeText(el.value);
}
function flash(id) {
  const el = document.getElementById(id);
  el.style.background = 'rgba(201,168,76,.15)';
  setTimeout(() => el.style.background = '', 400);
}

// ── Кнопка: лоадер ──
function setLoading(btnId, loading) {
  const btn = document.getElementById(btnId);
  if (loading) {
    btn._orig = btn.innerHTML;
    btn.innerHTML = '<span class="spinner"></span> Вычисляю...';
    btn.disabled = true;
  } else {
    btn.innerHTML = btn._orig;
    btn.disabled = false;
  }
}

// ── Shareable URL ──
function makeShareURL(seed) {
  return location.origin + '/?seed=' + encodeURIComponent(seed);
}
function setShareURL(inputId, seed) {
  document.getElementById(inputId).value = makeShareURL(seed);
}

// ── API: генерировать ──
async function doGenerate() {
  const seed = document.getElementById('seed-input').value;
  if (!seed.trim()) { setStatus('gen-status', 'Введите сид.', true); return; }
  clearStatus('gen-status');
  setLoading('btn-gen', true);
  try {
    const r = await fetch('/api/generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ seed })
    });
    const data = await r.json();
    if (!data.ok) { setStatus('gen-status', data.error, true); return; }
    document.getElementById('gen-page').textContent = data.page;
    document.getElementById('gen-result').style.display = '';
    document.getElementById('gen-share').style.display = '';
    setShareURL('gen-share-url', data.seed);
    setStatus('gen-status', '✓ Страница найдена', false);
  } catch(e) {
    setStatus('gen-status', 'Ошибка сети: ' + e.message, true);
  } finally {
    setLoading('btn-gen', false);
  }
}

// ── API: восстановить ──
async function doRecover() {
  const page = document.getElementById('page-input').value;
  const n = [...page].length;
  if (n !== 500) {
    setStatus('rec-status', 'Нужно ровно 500 символов, сейчас: ' + n, true);
    return;
  }
  clearStatus('rec-status');
  setLoading('btn-rec', true);
  try {
    const r = await fetch('/api/recover', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ page })
    });
    const data = await r.json();
    if (!data.ok) { setStatus('rec-status', data.error, true); return; }
    document.getElementById('rec-seed').textContent = data.seed;
    document.getElementById('rec-result').style.display = '';
    setStatus('rec-status', '✓ Адрес страницы восстановлен', false);
  } catch(e) {
    setStatus('rec-status', 'Ошибка сети: ' + e.message, true);
  } finally {
    setLoading('btn-rec', false);
  }
}

// ── API: случайная ──
async function doRandom() {
  clearStatus('rnd-status');
  setLoading('btn-rnd', true);
  try {
    const r = await fetch('/api/random');
    const data = await r.json();
    if (!data.ok) { setStatus('rnd-status', data.error, true); return; }
    document.getElementById('rnd-seed').textContent = data.seed;
    document.getElementById('rnd-page').textContent = data.page;
    document.getElementById('rnd-seed-result').style.display = '';
    document.getElementById('rnd-page-result').style.display = '';
    document.getElementById('rnd-share').style.display = '';
    setShareURL('rnd-share-url', data.seed);
    setStatus('rnd-status', '✓ Страница открыта из бесконечной библиотеки', false);
  } catch(e) {
    setStatus('rnd-status', 'Ошибка сети: ' + e.message, true);
  } finally {
    setLoading('btn-rnd', false);
  }
}

// ── Загрузка сида из URL ──
(async function() {
  const params = new URLSearchParams(location.search);
  const seed = params.get('seed');
  if (seed) {
    document.getElementById('seed-input').value = seed;
    updateSeedCount();
    await doGenerate();
  }
  // Загрузить реальные параметры с сервера
  try {
    const info = await fetch('/api/info').then(r => r.json());
    const ml = info.max_seed_len;
    document.getElementById('max-len').textContent = ml;
    document.getElementById('info-maxseed').textContent = ml;
    document.querySelector('#seed-input').maxLength = ml;
    window.MAX_SEED = ml;
  } catch(_) {}
})();
</script>
</body>
</html>`
