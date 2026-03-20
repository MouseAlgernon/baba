package main

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"
	"unicode/utf8"
)

// ============================================================
// НАСТРОЙКИ
// ============================================================

const PAGE_SIZE = 500 // размер страницы в символах

// ============================================================
// Инициализация
// ============================================================

var (
	PAGE_ALPHABET []rune
	pageIndex     map[rune]int
	MAX_SEED_LEN  int      // макс. длина сида в рунах
	pageSpaceSize *big.Int // |PAGE_ALPHABET|^PAGE_SIZE
)

const UNICODE_BASE = 1114112 // весь Unicode (U+0000 – U+10FFFF)

func init() {
	PAGE_ALPHABET = buildPageAlphabet()
	pageIndex = makeRuneIndex(PAGE_ALPHABET)

	pageBase := big.NewInt(int64(len(PAGE_ALPHABET)))
	pageSpaceSize = new(big.Int).Exp(pageBase, big.NewInt(int64(PAGE_SIZE)), nil)

	// Макс. длина сида: наибольшее L при котором UNICODE_BASE^L < pageSpaceSize
	MAX_SEED_LEN = int(float64(PAGE_SIZE)*math.Log(float64(len(PAGE_ALPHABET)))/math.Log(UNICODE_BASE))
	ub := big.NewInt(UNICODE_BASE)
	for {
		limit := new(big.Int).Exp(ub, big.NewInt(int64(MAX_SEED_LEN+1)), nil)
		if limit.Cmp(pageSpaceSize) >= 0 {
			break
		}
		MAX_SEED_LEN++
	}
}

func buildPageAlphabet() []rune {
	var chars []rune
	for r := rune(0x0430); r <= 0x044F; r++ { chars = append(chars, r) } // а-я
	for r := rune(0x0410); r <= 0x042F; r++ { chars = append(chars, r) } // А-Я
	chars = append(chars, 'ё', 'Ё')
	chars = append(chars, []rune{
		' ', '\n', '.', ',', '!', '?', ';', ':', '-',
		'–', '—', '(', ')', '«', '»', '"', '\'', '…', '/', '\\',
	}...)
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
//
// Сид — любой Unicode текст длиной ≤ MAX_SEED_LEN (159 рун).
// Каждая руна — кодпоинт 0..1114111, то есть цифра в base-1114112.
// Число однозначно определяет страницу и наоборот.
// ============================================================

func seedToNumber(seed string) (*big.Int, error) {
	runes := []rune(seed)
	if len(runes) == 0 {
		return nil, fmt.Errorf("сид не может быть пустым")
	}
	if len(runes) > MAX_SEED_LEN {
		return nil, fmt.Errorf(
			"сид слишком длинный: %d символов, максимум %d",
			len(runes), MAX_SEED_LEN)
	}
	base := big.NewInt(UNICODE_BASE)
	result := big.NewInt(0)
	for _, r := range runes {
		if r < 0 || int64(r) >= UNICODE_BASE {
			return nil, fmt.Errorf("недопустимый символ: U+%04X", r)
		}
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
		return string(rune(0)) // U+0000
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

func randomSeed() (string, error) {
	ub := big.NewInt(UNICODE_BASE)
	chars := make([]rune, MAX_SEED_LEN)
	for i := range chars {
		n, err := rand.Int(rand.Reader, ub)
		if err != nil {
			return "", err
		}
		chars[i] = rune(n.Int64())
	}
	return string(chars), nil
}

// ============================================================
// UI
// ============================================================

var scanner = bufio.NewReader(os.Stdin)

func readLine(prompt string) string {
	fmt.Print(prompt)
	line, _ := scanner.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

func readPage() ([]rune, error) {
	fmt.Printf("Введите страницу (%d символов). Завершите строкой: <<<END>>>\n", PAGE_SIZE)
	var sb strings.Builder
	for {
		line, _ := scanner.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if line == "<<<END>>>" {
			break
		}
		if sb.Len() > 0 {
			sb.WriteRune('\n')
		}
		sb.WriteString(line)
	}
	runes := []rune(sb.String())
	if len(runes) != PAGE_SIZE {
		return nil, fmt.Errorf("ожидалось %d символов, получено %d", PAGE_SIZE, len(runes))
	}
	return runes, nil
}

func sep(title string) {
	line := strings.Repeat("═", 54)
	fmt.Printf("\n%s\n  %s\n%s\n", line, title, line)
}

func printMenu() {
	fmt.Printf(`
╔══════════════════════════════════════════════════════╗
║             ВАВИЛОНСКАЯ БИБЛИОТЕКА (Go)              ║
║  Страница      : %4d символов                       ║
║  Алфавит стр.  : %4d символов (рус. + пунктуация)  ║
║  Алфавит сида  : любой Unicode (все языки, эмодзи)  ║
║  Макс. сид     : %4d символов  [строго 1-в-1]       ║
╠══════════════════════════════════════════════════════╣
║  1. Генерировать страницу по сиду                    ║
║  2. Восстановить сид по странице                     ║
║  3. Случайная страница                               ║
║  4. Показать алфавит страницы                        ║
║  0. Выход                                            ║
╚══════════════════════════════════════════════════════╝
`, PAGE_SIZE, len(PAGE_ALPHABET), MAX_SEED_LEN)
	fmt.Print("Выбор: ")
}

func actionGenerate() {
	seed := readLine(fmt.Sprintf("Введите сид (любой Unicode, макс. %d символов): ", MAX_SEED_LEN))
	if strings.TrimSpace(seed) == "" {
		fmt.Println("Ошибка: сид не может быть пустым.")
		return
	}
	runes := []rune(seed)
	if len(runes) > MAX_SEED_LEN {
		fmt.Printf("Сид обрезан до %d символов.\n", MAX_SEED_LEN)
		seed = string(runes[:MAX_SEED_LEN])
	}
	n, err := seedToNumber(seed)
	if err != nil {
		fmt.Println("Ошибка:", err)
		return
	}
	page := numberToPage(n)
	sep("СТРАНИЦА")
	fmt.Println(string(page))
	sep(fmt.Sprintf("Символов: %d | Байт UTF-8: %d",
		utf8.RuneCountInString(string(page)), len([]byte(string(page)))))
}

func actionRecover() {
	page, err := readPage()
	if err != nil {
		fmt.Println("Ошибка:", err)
		return
	}
	n, err := pageToNumber(page)
	if err != nil {
		fmt.Println("Ошибка:", err)
		return
	}
	seed := numberToSeed(n)
	sep("СИД")
	fmt.Printf("%q\n", seed) // %q показывает управляющие символы экранированными
	fmt.Println(seed)        // также выводим как есть

	// Проверка
	n2, err2 := seedToNumber(seed)
	if err2 == nil && string(numberToPage(n2)) == string(page) {
		fmt.Println("\n✓ Проверка: УСПЕХ (строгая биекция)")
	} else {
		fmt.Println("\n✗ Ошибка обратимости")
	}
}

func actionRandom() {
	seed, err := randomSeed()
	if err != nil {
		fmt.Println("Ошибка:", err)
		return
	}
	n, err := seedToNumber(seed)
	if err != nil {
		fmt.Println("Ошибка:", err)
		return
	}
	page := numberToPage(n)
	sep(fmt.Sprintf("СЛУЧАЙНЫЙ СИД (%d символов Unicode)", len([]rune(seed))))
	fmt.Printf("%q\n", seed)
	sep("СТРАНИЦА")
	fmt.Println(string(page))
	fmt.Println("\n(Вставьте страницу в пункт 2 — получите этот же сид)")
}

func actionShowAlphabet() {
	sep(fmt.Sprintf("АЛФАВИТ СТРАНИЦЫ (%d символов)", len(PAGE_ALPHABET)))
	for i, r := range PAGE_ALPHABET {
		switch r {
		case '\n':
			fmt.Printf("  [%3d] U+%04X  <перенос строки>\n", i, r)
		case ' ':
			fmt.Printf("  [%3d] U+%04X  <пробел>\n", i, r)
		default:
			fmt.Printf("  [%3d] U+%04X  %c\n", i, r, r)
		}
	}
	fmt.Printf("\nАлфавит сида: весь Unicode (%d кодпоинтов)\n", UNICODE_BASE)
	fmt.Printf("Максимальная длина сида: %d символов\n", MAX_SEED_LEN)
	fmt.Printf("Пространство страниц: %d^%d\n", len(PAGE_ALPHABET), PAGE_SIZE)
}

func main() {
	fmt.Printf("Инициализация... алфавит=%d, макс.сид=%d символов Unicode\n",
		len(PAGE_ALPHABET), MAX_SEED_LEN)
	for {
		printMenu()
		choice, _ := scanner.ReadString('\n')
		choice = strings.TrimSpace(choice)
		switch choice {
		case "1":
			actionGenerate()
		case "2":
			actionRecover()
		case "3":
			actionRandom()
		case "4":
			actionShowAlphabet()
		case "0":
			fmt.Println("\nДо свидания!")
			return
		default:
			fmt.Println("Неверный выбор.")
		}
	}
}
