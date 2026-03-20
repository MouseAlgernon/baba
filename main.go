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
const UNICODE_BASE = 1114112 // весь Unicode U+0000–U+10FFFF

var (
	PAGE_ALPHABET []rune
	pageIndex     map[rune]int
	MAX_SEED_LEN  int
	pageSpaceSize *big.Int
)

func init() {
	PAGE_ALPHABET = buildPageAlphabet()
	pageIndex = makeRuneIndex(PAGE_ALPHABET)

	pageBase := big.NewInt(int64(len(PAGE_ALPHABET)))
	pageSpaceSize = new(big.Int).Exp(pageBase, big.NewInt(int64(PAGE_SIZE)), nil)

	MAX_SEED_LEN = int(float64(PAGE_SIZE)*math.Log(float64(len(PAGE_ALPHABET)))/math.Log(UNICODE_BASE))
	ub := big.NewInt(UNICODE_BASE)
	for {
		limit := new(big.Int).Exp(ub, big.NewInt(int64(MAX_SEED_LEN+1)), nil)
		if limit.Cmp(pageSpaceSize) >= 0 {
			break
		}
		MAX_SEED_LEN++
	}
	log.Printf("Babylon ready: page_alpha=%d, max_seed=%d unicode chars", len(PAGE_ALPHABET), MAX_SEED_LEN)
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

func makeRuneIndex(alphabet []rune) map[rune]int {
	m := make(map[rune]int, len(alphabet))
	for i, r := range alphabet { m[r] = i }
	return m
}

// ============================================================
// Биективная арифметика — сид в base-1114112 (Unicode)
// ============================================================

func seedToNumber(seed string) (*big.Int, error) {
	runes := []rune(seed)
	if len(runes) == 0 {
		return nil, fmt.Errorf("сид не может быть пустым")
	}
	if len(runes) > MAX_SEED_LEN {
		return nil, fmt.Errorf("сид слишком длинный: %d символов, максимум %d", len(runes), MAX_SEED_LEN)
	}
	base := big.NewInt(UNICODE_BASE)
	result := big.NewInt(0)
	for _, r := range runes {
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(r)))
	}
	if result.Cmp(pageSpaceSize) >= 0 {
		return nil, fmt.Errorf("сид выходит за пределы пространства страниц")
	}
	return result, nil
}

func numberToSeed(n *big.Int) string {
	if n.Sign() == 0 {
		return string(rune(0))
	}
	base := big.NewInt(UNICODE_BASE)
	tmp := new(big.Int).Set(n)
	mod := new(big.Int)
	var chars []rune
	for tmp.Sign() > 0 {
		tmp.DivMod(tmp, base, mod)
		chars = append(chars, rune(mod.Int64()))
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
	ub := big.NewInt(UNICODE_BASE)
	chars := make([]rune, MAX_SEED_LEN)
	for i := range chars {
		n, err := rand.Int(rand.Reader, ub)
		if err != nil { return "", err }
		chars[i] = rune(n.Int64())
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

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { writeJSON(w, 405, Response{Error: "метод не разрешён"}); return }
	var req struct{ Seed string `json:"seed"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeJSON(w, 400, Response{Error: "неверный JSON"}); return }

	seed := req.Seed
	// Автообрезка до MAX_SEED_LEN
	if runes := []rune(seed); len(runes) > MAX_SEED_LEN {
		seed = string(runes[:MAX_SEED_LEN])
	}
	seed = strings.TrimSpace(seed)
	if seed == "" { writeJSON(w, 400, Response{Error: "сид не может быть пустым"}); return }

	n, err := seedToNumber(seed)
	if err != nil { writeJSON(w, 400, Response{Error: err.Error()}); return }
	page := numberToPage(n)
	writeJSON(w, 200, Response{OK: true, Page: string(page), Seed: seed})
}

func handleRecover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { writeJSON(w, 405, Response{Error: "метод не разрешён"}); return }
	var req struct{ Page string `json:"page"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeJSON(w, 400, Response{Error: "неверный JSON"}); return }

	runes := []rune(req.Page)
	if len(runes) != PAGE_SIZE {
		writeJSON(w, 400, Response{Error: fmt.Sprintf("нужно ровно %d символов, получено %d", PAGE_SIZE, len(runes))})
		return
	}
	n, err := pageToNumber(runes)
	if err != nil { writeJSON(w, 400, Response{Error: err.Error()}); return }
	seed := numberToSeed(n)
	writeJSON(w, 200, Response{OK: true, Seed: seed, Page: req.Page})
}

func handleRandom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet { writeJSON(w, 405, Response{Error: "метод не разрешён"}); return }
	seed, err := generateRandomSeed()
	if err != nil { writeJSON(w, 500, Response{Error: "ошибка генерации"}); return }
	n, err := seedToNumber(seed)
	if err != nil { writeJSON(w, 500, Response{Error: err.Error()}); return }
	page := numberToPage(n)
	writeJSON(w, 200, Response{OK: true, Seed: seed, Page: string(page)})
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]any{
		"page_size":       PAGE_SIZE,
		"page_alpha_size": len(PAGE_ALPHABET),
		"seed_alpha":      "Unicode (все языки, эмодзи, спецсимволы)",
		"max_seed_len":    MAX_SEED_LEN,
	})
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(indexHTML))
}

func main() {
	port := os.Getenv("PORT")
	if port == "" { port = "8080" }
	mux := http.NewServeMux()
	mux.HandleFunc("/api/generate", handleGenerate)
	mux.HandleFunc("/api/recover", handleRecover)
	mux.HandleFunc("/api/random", handleRandom)
	mux.HandleFunc("/api/info", handleInfo)
	mux.HandleFunc("/", handleIndex)
	log.Printf("Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

const indexHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Вавилонская Библиотека</title>
<style>
  :root {
    --bg:#0d0b08; --surface:#141209; --border:#2a2418;
    --gold:#c9a84c; --gold-dim:#7a6128; --text:#d4c9a8;
    --muted:#6b5f42; --red:#c0392b; --green:#27ae60;
    --mono:'Courier New',Courier,monospace;
    --serif:Georgia,'Times New Roman',serif;
  }
  *{box-sizing:border-box;margin:0;padding:0}
  body{background:var(--bg);color:var(--text);font-family:var(--serif);min-height:100vh;display:flex;flex-direction:column}
  header{border-bottom:1px solid var(--border);padding:2rem 2rem 1.5rem;text-align:center}
  header h1{font-size:clamp(1.4rem,4vw,2.2rem);color:var(--gold);letter-spacing:.12em;text-transform:uppercase}
  header p{margin-top:.5rem;color:var(--muted);font-size:.85rem;letter-spacing:.05em}
  nav{display:flex;border-bottom:1px solid var(--border)}
  nav button{flex:1;padding:.9rem .5rem;background:none;border:none;border-bottom:2px solid transparent;color:var(--muted);font-family:var(--serif);font-size:.9rem;letter-spacing:.06em;cursor:pointer;transition:color .2s,border-color .2s}
  nav button:hover{color:var(--text)}
  nav button.active{color:var(--gold);border-bottom-color:var(--gold)}
  main{flex:1;max-width:860px;width:100%;margin:0 auto;padding:2rem 1.5rem}
  .panel{display:none} .panel.active{display:block}
  label{display:block;color:var(--gold-dim);font-size:.75rem;letter-spacing:.1em;text-transform:uppercase;margin-bottom:.4rem}
  input[type=text],textarea{width:100%;background:var(--surface);border:1px solid var(--border);border-radius:3px;color:var(--text);font-family:var(--mono);font-size:.82rem;padding:.7rem .9rem;outline:none;resize:vertical;transition:border-color .2s}
  input[type=text]:focus,textarea:focus{border-color:var(--gold-dim)}
  .char-count{text-align:right;font-size:.72rem;color:var(--muted);margin-top:.25rem}
  .char-count.over{color:var(--red)}
  .hint{font-size:.75rem;color:var(--muted);margin-top:.35rem;font-family:var(--serif)}
  .btn{display:inline-flex;align-items:center;gap:.5rem;margin-top:1rem;padding:.65rem 1.5rem;background:none;border:1px solid var(--gold-dim);border-radius:3px;color:var(--gold);font-family:var(--serif);font-size:.9rem;letter-spacing:.06em;cursor:pointer;transition:background .2s,border-color .2s}
  .btn:hover{background:rgba(201,168,76,.08);border-color:var(--gold)}
  .btn:disabled{opacity:.4;cursor:not-allowed}
  .btn-row{display:flex;gap:.75rem;flex-wrap:wrap}
  .result{margin-top:1.75rem;border:1px solid var(--border);border-radius:3px;overflow:hidden}
  .result-header{background:var(--surface);padding:.55rem 1rem;display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid var(--border)}
  .result-header span{font-size:.72rem;letter-spacing:.1em;text-transform:uppercase;color:var(--gold-dim)}
  .copy-btn{background:none;border:1px solid var(--border);border-radius:2px;color:var(--muted);font-size:.72rem;padding:.2rem .6rem;cursor:pointer;transition:color .2s,border-color .2s}
  .copy-btn:hover{color:var(--gold);border-color:var(--gold-dim)}
  .result-body{padding:1rem;font-family:var(--mono);font-size:.82rem;line-height:1.7;white-space:pre-wrap;word-break:break-all;max-height:360px;overflow-y:auto}
  .status{margin-top:1rem;padding:.5rem .9rem;border-radius:3px;font-size:.82rem;display:none}
  .status.ok{background:rgba(39,174,96,.12);border:1px solid rgba(39,174,96,.3);color:var(--green);display:block}
  .status.err{background:rgba(192,57,43,.12);border:1px solid rgba(192,57,43,.3);color:var(--red);display:block}
  .share-row{margin-top:.75rem;display:flex;gap:.5rem;align-items:center}
  .share-row input{flex:1;font-size:.75rem;padding:.4rem .7rem}
  .info-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:1rem;margin-top:1rem}
  .info-card{background:var(--surface);border:1px solid var(--border);border-radius:3px;padding:1rem;text-align:center}
  .info-card .val{font-size:1.6rem;color:var(--gold);font-family:var(--mono)}
  .info-card .lbl{margin-top:.3rem;font-size:.72rem;color:var(--muted);letter-spacing:.06em;text-transform:uppercase}
  .spinner{display:inline-block;width:1em;height:1em;border:2px solid var(--gold-dim);border-top-color:var(--gold);border-radius:50%;animation:spin .6s linear infinite;vertical-align:middle}
  @keyframes spin{to{transform:rotate(360deg)}}
  footer{border-top:1px solid var(--border);padding:1rem;text-align:center;color:var(--muted);font-size:.75rem;letter-spacing:.06em}
  @media(max-width:500px){main{padding:1.2rem 1rem}nav button{font-size:.78rem;padding:.7rem .3rem}}
</style>
</head>
<body>
<header>
  <h1>⟁ Вавилонская Библиотека</h1>
  <p>Каждая возможная страница существует. Найди свою.</p>
</header>
<nav>
  <button class="active" onclick="switchTab('generate',this)">Генерировать</button>
  <button onclick="switchTab('recover',this)">Восстановить</button>
  <button onclick="switchTab('random',this)">Случайная</button>
  <button onclick="switchTab('about',this)">О библиотеке</button>
</nav>
<main>

  <!-- ГЕНЕРИРОВАТЬ -->
  <div class="panel active" id="tab-generate">
    <label>Сид — адрес страницы</label>
    <input type="text" id="seed-input"
      placeholder="Любой текст: русский, English, 中文, эмодзи 🌍, символы !@#$..."
      oninput="updateSeedCount()" onkeydown="if(event.key==='Enter')doGenerate()">
    <div class="char-count" id="seed-count">0 / 159</div>
    <div class="hint">Принимается любой текст на любом языке, до <strong id="max-len-hint">159</strong> символов. Более длинный сид обрезается автоматически.</div>
    <div class="btn-row">
      <button class="btn" onclick="doGenerate()" id="btn-gen">Открыть страницу</button>
    </div>
    <div class="status" id="gen-status"></div>
    <div class="result" id="gen-result" style="display:none">
      <div class="result-header"><span>Страница</span><button class="copy-btn" onclick="copyText('gen-page')">Копировать</button></div>
      <div class="result-body" id="gen-page"></div>
    </div>
    <div class="share-row" id="gen-share" style="display:none">
      <input type="text" id="gen-share-url" readonly onclick="this.select()">
      <button class="copy-btn" onclick="copyField('gen-share-url')">Скопировать ссылку</button>
    </div>
  </div>

  <!-- ВОССТАНОВИТЬ -->
  <div class="panel" id="tab-recover">
    <label>Вставьте страницу (ровно 500 символов)</label>
    <textarea id="page-input" rows="10"
      placeholder="Вставьте текст страницы сюда..."
      oninput="updatePageCount()"></textarea>
    <div class="char-count" id="page-count">0 / 500</div>
    <div class="btn-row">
      <button class="btn" onclick="doRecover()" id="btn-rec">Найти адрес</button>
    </div>
    <div class="status" id="rec-status"></div>
    <div class="result" id="rec-result" style="display:none">
      <div class="result-header"><span>Сид (адрес)</span><button class="copy-btn" onclick="copyText('rec-seed')">Копировать</button></div>
      <div class="result-body" id="rec-seed"></div>
    </div>
  </div>

  <!-- СЛУЧАЙНАЯ -->
  <div class="panel" id="tab-random">
    <p style="color:var(--muted);font-size:.9rem;line-height:1.7;margin-bottom:1.2rem">
      Случайная страница выбирается из 86<sup>500</sup> возможных.
      Вероятность получить одну и ту же страницу дважды ничтожно мала.
    </p>
    <div class="btn-row">
      <button class="btn" onclick="doRandom()" id="btn-rnd">Открыть случайную страницу</button>
    </div>
    <div class="status" id="rnd-status"></div>
    <div class="result" id="rnd-seed-result" style="display:none">
      <div class="result-header"><span>Адрес (сид)</span><button class="copy-btn" onclick="copyText('rnd-seed')">Копировать</button></div>
      <div class="result-body" id="rnd-seed"></div>
    </div>
    <div class="result" id="rnd-page-result" style="display:none">
      <div class="result-header"><span>Страница</span><button class="copy-btn" onclick="copyText('rnd-page')">Копировать</button></div>
      <div class="result-body" id="rnd-page"></div>
    </div>
    <div class="share-row" id="rnd-share" style="display:none">
      <input type="text" id="rnd-share-url" readonly onclick="this.select()">
      <button class="copy-btn" onclick="copyField('rnd-share-url')">Скопировать ссылку</button>
    </div>
  </div>

  <!-- О БИБЛИОТЕКЕ -->
  <div class="panel" id="tab-about">
    <div class="info-grid">
      <div class="info-card"><div class="val">500</div><div class="lbl">Символов на странице</div></div>
      <div class="info-card"><div class="val">86</div><div class="lbl">Символов в алфавите</div></div>
      <div class="info-card"><div class="val">∞*</div><div class="lbl">Языков для сида</div></div>
      <div class="info-card"><div class="val" id="info-maxseed">159</div><div class="lbl">Макс. длина сида</div></div>
    </div>
    <div style="margin-top:1.75rem;line-height:1.8;color:var(--text);font-size:.92rem">
      <p style="margin-bottom:1rem">
        <strong style="color:var(--gold)">Вавилонская библиотека</strong> — концепция Хорхе Луиса Борхеса:
        библиотека, содержащая все возможные книги. Эта реализация охватывает все страницы
        из 500 символов русского алфавита, знаков препинания и переноса строки.
      </p>
      <p style="margin-bottom:1rem">
        <strong style="color:var(--gold)">Сид — это адрес.</strong> Любой текст на любом языке
        (русский, английский, китайский, эмодзи, математические символы) длиной до 159 символов
        однозначно указывает на конкретную страницу. Каждый символ — это кодпоинт Unicode,
        число в системе счисления с основанием 1 114 112.
      </p>
      <p>
        <strong style="color:var(--gold)">Строгая биекция.</strong> Никаких хешей, никакой базы данных.
        Страница вычисляется из сида за миллисекунды, сид восстанавливается из страницы — и это всегда
        тот же самый сид. Каждой из 86<sup>500</sup> страниц соответствует ровно один адрес.
      </p>
    </div>
  </div>

</main>
<footer>Вавилонская Библиотека · 86<sup>500</sup> страниц · каждая уникальна</footer>

<script>
let MAX_SEED = 159;

function switchTab(name, btn) {
  document.querySelectorAll('.panel').forEach(p=>p.classList.remove('active'));
  document.querySelectorAll('nav button').forEach(b=>b.classList.remove('active'));
  document.getElementById('tab-'+name).classList.add('active');
  btn.classList.add('active');
}

function updateSeedCount() {
  const el = document.getElementById('seed-input');
  const n = [...el.value].length;
  const cnt = document.getElementById('seed-count');
  cnt.textContent = n + ' / ' + MAX_SEED;
  cnt.classList.toggle('over', n > MAX_SEED);
}
function updatePageCount() {
  const el = document.getElementById('page-input');
  const n = [...el.value].length;
  const cnt = document.getElementById('page-count');
  cnt.textContent = n + ' / 500';
  cnt.classList.toggle('over', n !== 500);
}

function setStatus(id, msg, isErr) {
  const el = document.getElementById(id);
  el.textContent = msg;
  el.className = 'status ' + (isErr ? 'err' : 'ok');
}
function clearStatus(id) { document.getElementById(id).className='status'; }

function copyText(id) {
  navigator.clipboard.writeText(document.getElementById(id).textContent);
}
function copyField(id) {
  const el = document.getElementById(id);
  el.select();
  navigator.clipboard.writeText(el.value);
}

function setLoading(btnId, loading) {
  const btn = document.getElementById(btnId);
  if (loading) { btn._orig=btn.innerHTML; btn.innerHTML='<span class="spinner"></span> Вычисляю...'; btn.disabled=true; }
  else { btn.innerHTML=btn._orig; btn.disabled=false; }
}

function makeShareURL(seed) {
  return location.origin + '/?seed=' + encodeURIComponent(seed);
}

async function doGenerate() {
  const seed = document.getElementById('seed-input').value;
  if (!seed.trim()) { setStatus('gen-status','Введите сид.',true); return; }
  clearStatus('gen-status');
  setLoading('btn-gen', true);
  try {
    const r = await fetch('/api/generate', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({seed})
    });
    const data = await r.json();
    if (!data.ok) { setStatus('gen-status', data.error, true); return; }
    // Обновляем поле сида если он был обрезан
    document.getElementById('seed-input').value = data.seed;
    updateSeedCount();
    document.getElementById('gen-page').textContent = data.page;
    document.getElementById('gen-result').style.display = '';
    document.getElementById('gen-share').style.display = '';
    document.getElementById('gen-share-url').value = makeShareURL(data.seed);
    setStatus('gen-status', '✓ Страница найдена', false);
  } catch(e) {
    setStatus('gen-status', 'Ошибка сети: '+e.message, true);
  } finally { setLoading('btn-gen', false); }
}

async function doRecover() {
  const page = document.getElementById('page-input').value;
  const n = [...page].length;
  if (n !== 500) { setStatus('rec-status','Нужно ровно 500 символов, сейчас: '+n,true); return; }
  clearStatus('rec-status');
  setLoading('btn-rec', true);
  try {
    const r = await fetch('/api/recover', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({page})
    });
    const data = await r.json();
    if (!data.ok) { setStatus('rec-status', data.error, true); return; }
    document.getElementById('rec-seed').textContent = data.seed;
    document.getElementById('rec-result').style.display = '';
    setStatus('rec-status', '✓ Адрес страницы восстановлен', false);
  } catch(e) {
    setStatus('rec-status', 'Ошибка сети: '+e.message, true);
  } finally { setLoading('btn-rec', false); }
}

async function doRandom() {
  clearStatus('rnd-status');
  setLoading('btn-rnd', true);
  try {
    const data = await fetch('/api/random').then(r=>r.json());
    if (!data.ok) { setStatus('rnd-status', data.error, true); return; }
    document.getElementById('rnd-seed').textContent = data.seed;
    document.getElementById('rnd-page').textContent = data.page;
    document.getElementById('rnd-seed-result').style.display = '';
    document.getElementById('rnd-page-result').style.display = '';
    document.getElementById('rnd-share').style.display = '';
    document.getElementById('rnd-share-url').value = makeShareURL(data.seed);
    setStatus('rnd-status', '✓ Страница открыта из бесконечной библиотеки', false);
  } catch(e) {
    setStatus('rnd-status', 'Ошибка сети: '+e.message, true);
  } finally { setLoading('btn-rnd', false); }
}

// Загрузка сида из URL и реальных параметров с сервера
(async function(){
  try {
    const info = await fetch('/api/info').then(r=>r.json());
    MAX_SEED = info.max_seed_len;
    document.getElementById('max-len-hint').textContent = MAX_SEED;
    document.getElementById('info-maxseed').textContent = MAX_SEED;
    document.getElementById('seed-count').textContent = '0 / ' + MAX_SEED;
  } catch(_){}

  const seed = new URLSearchParams(location.search).get('seed');
  if (seed) {
    document.getElementById('seed-input').value = seed;
    updateSeedCount();
    await doGenerate();
  }
})();
</script>
</body>
</html>`
