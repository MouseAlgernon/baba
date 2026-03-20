# ⟁ Вавилонская Библиотека

Веб-версия Вавилонской библиотеки — 86^500 уникальных страниц на русском языке.
Каждая страница вычисляется из адреса (сида) и наоборот. Никакой базы данных.

## Структура

```
babylon/
├── main.go       — весь бэкенд + встроенный HTML (один файл)
├── go.mod
└── render.yaml   — конфиг для Render
```

## Деплой на Render (бесплатно, 5 минут)

1. Залей папку `babylon/` в новый GitHub репозиторий:
   ```bash
   cd babylon
   git init
   git add .
   git commit -m "babylon library"
   git remote add origin https://github.com/ВАШ_НИК/babylon.git
   git push -u origin main
   ```

2. Зайди на https://render.com → **New** → **Web Service**

3. Подключи GitHub репозиторий

4. Render сам обнаружит `render.yaml` и настроит всё автоматически:
   - Runtime: **Go**
   - Build: `go build -o babylon .`
   - Start: `./babylon`
   - Port: `10000`

5. Нажми **Deploy** — через 2–3 минуты сайт живёт на `https://babylon-library.onrender.com`

## Локальный запуск

```bash
cd babylon
go run .
# откроется на http://localhost:8080
```

## API

| Метод | URL | Тело / Ответ |
|---|---|---|
| POST | `/api/generate` | `{"seed":"..."}` → `{"ok":true,"page":"...","seed":"..."}` |
| POST | `/api/recover`  | `{"page":"..."}` → `{"ok":true,"seed":"...","page":"..."}` |
| GET  | `/api/random`   | → `{"ok":true,"seed":"...","page":"..."}` |
| GET  | `/api/info`     | → `{"page_size":500,"max_seed_len":489,...}` |

## Как работает

Страница (500 символов, алфавит 86) — это число в системе счисления 86.
Сид (алфавит ASCII 32–126, 95 символов) — то же число в системе счисления 95.

Строгая биекция без mod и без хешей:
- `seed → число → страница`  
- `страница → число → seed` (тот же самый)

Максимальная длина сида: **489 символов** (95^489 < 86^500 ≤ 95^490).
