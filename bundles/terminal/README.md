# Bundle terminalowy srcpack

Ten katalog można skopiować w całości do innego projektu Go. Nie importuje pakietów srcpack, konfiguracji workspace, parsera ani CLI. Zawiera jednak **obecną politykę wyświetlania srcpack** — etykiety `SRCPACK`, legendę drzewa, progi kolorów i układ raportów. To osobny bundle na potrzeby produktu, a nie jeszcze ogólny moduł Terminal.

`tree.nflow` odpowiada za nagłówki, legendę, ikony ról, metadane, procenty, kolory, paski i stopkę drzewa. `summary.nflow` przygotowuje model widoku: kolejność i wartości pól, wartości domyślne, skracanie tekstu oraz dane opcjonalnych szacunków. Układ raportu znajduje się w sąsiednim `summary.tmpl` i jest renderowany przez istniejący renderer Flow `template.render`. Zmiany danych i reguł przygotowania wykonuje się w NFlow, a zmiany układu — w pliku `.tmpl`, nie w adapterach Go.

Go dostarcza jedynie ogólne prymitywy: `terminal.printf`, `width`, `tokens`, `bytes`, `size`, `age`, `clean`, `paint`, `bar` oraz dostęp do osadzonych plików `.tmpl`. Szablony są osadzone z tego samego katalogu co źródła NFlow. Kolory i progi wybiera NFlow. Bundle nie wykrywa TTY, nie odczytuje zegara ani plików hosta, nie zapisuje stdout/stderr i nie uruchamia animacji. Operacje systemowe pozostają u hosta.

## Najprostsze użycie z Go

Skopiuj cały katalog, zmień import w aplikacji na własną ścieżkę i użyj Go 1.27.3 oraz Flow v0.64.0/kernel v0.36.0:

```go
import (
    "context"
    "example.com/myapp/bundles/terminal"
)

text, err := terminal.Summary(context.Background(), map[string]any{
    "config": map[string]any{"profile": "arch", "editor": "markdown"},
    "stats": map[string]any{
        "file_count": 2, "func_count": 4,
        "raw_bytes": int64(4096), "packed_bytes": int64(1024),
        "estimated_tokens": int64(292), "saved_percent": 75.0,
    },
    "elapsed": "25ms", "color": false,
    "clipboard_requested": false, "tree": "",
})
// Host obsługuje err i sam decyduje, gdzie zapisać text.
```

`terminal.Tree(ctx, input)` działa analogicznie. Obie funkcje zwracają `(string, error)`, używają źródeł osadzonych z tego katalogu i propagują błędy oraz anulowanie kontekstu. Konfiguracja runnera jest współdzielona bezpiecznie między wywołaniami; źródło jest wykonywane dla każdego przekazanego zestawu faktów.

## Komponowanie w NFlow

Przy bezpośrednim uruchamianiu NFlow zamontuj `terminal.Bundle(nil)` i `template.Bundle(nil)` obok `native.Bundles()` oraz zainstaluj `terminal.Loader()` przez `compiler.WithSourceLoader`. `terminal.Summary` montuje renderer szablonów we własnym runnerze. W większej aplikacji skomponuj loadery; `summary.tmpl` jest odczytywany z osadzonych zasobów bundle, a `@require terminal` i `@require template` nie zastępują dołączenia źródła.

```nflow
@include "summary.nflow"
@pipeline moj_raport
  @terminal_summary_format()
@end
pipeline.moj_raport
```

Dostępne makra to `@terminal_summary_format()` i `@terminal_tree_format()`, a gotowe pipeline’y to `terminal_summary` i `terminal_tree`. Makro podsumowania przygotowuje dane w NFlow i wywołuje zamontowaną akcję `template.render`; Flow macros mogą komponować akcje innych zamontowanych bibliotek, ale nie rejestrują ani nie montują bibliotek. Makra przyjmują pojedynczą wartość, nie strumień — przed nimi użyj `collect`, jeśli wejście jest strumieniowe. Wynikiem jest `{content: string}`. Przy ręcznym składaniu bundle trzeba zachować **zarówno `Libraries`, jak i `Symbols`**.

## Kontrakt raportu

`stats` zawiera liczby `file_count`, `func_count`, `raw_bytes`, `packed_bytes`, `estimated_tokens`, `saved_percent`. Wartości `config` są opcjonalne: tekstowe `profile`, `targets`, `prompt`, `editor`, `directory`, `output`, bool `cost` oraz tablica `pricing`. Każdy element `pricing` ma `label`, `currency` i `per_million_tokens` (liczba jednostek danej waluty na milion szacowanych zaoszczędzonych tokenów). Pozostałe pola wejścia to teksty `elapsed` i `tree` oraz bool `color` i `clipboard_requested`.

Host dostarcza czas jako gotową etykietę. `clipboard_requested` oznacza żądanie użytkownika, a nie potwierdzenie późniejszego skopiowania. Nie znaleziono w projekcie rzeczywistego katalogu cen i bundle nie zawiera domyślnych nazw modeli ani stawek. Gdy `cost` jest włączone, raport pokazuje szacowaną liczbę zaoszczędzonych tokenów; kwoty pojawiają się wyłącznie dla stawek dostarczonych przez hosta w `config.pricing` i są oznaczone jako szacunki. Raport nie przedstawia dostarczonych stawek jako aktualnego katalogu.

## Kontrakt drzewa

Host dostarcza:

- `empty`, `full`, `color`: bool;
- `included`, `excluded`, `total_files`, `total_tokens`, `raw_bytes`, `packed_bytes`: liczby całkowite;
- `languages`: mapa nazwa języka → liczba plików;
- `rows`: tablica widocznych wierszy, już uporządkowanych i ograniczonych głębokością przez hosta.

Każdy wiersz ma wszystkie poniższe pola:

```go
map[string]any{
    "name": "src", "dir": true, "prefix": "", "connector": "└── ",
    "files": 1, "tokens": int64(100), "funcs": 2, "lang": "go",
    "fully_excluded": false, "role": "", "age": -1, "lines": 0,
    "paired": "", "excluded": false, "target": false,
}
```

`age: -1` oznacza brak informacji o czasie; `0` oznacza dzisiaj. Role to `test`, `config`, `doc`, `gen`, `entry` albo pusty tekst. Pola agregatów dotyczą katalogów, a wiek, liczba linii, rola i parowanie — plików. Niepotrzebne pola nadal należy podać z wartościami zerowymi/pustymi. Formatowanie nie wykonuje ponownego spaceru po dysku.

## Zgodność i ograniczenia

Domyślne `tree-depth=1` nadal rozstrzyga srcpack, poza tym bundle. Zachowane są bajty dotychczasowego raportu i drzewa, z wyjątkiem wcześniej losowych remisów języków: kolejność i język dominujący są teraz rozstrzygane alfabetycznie. Pełne drzewo zachowuje także historyczną semantykę liczb w stopce; nie zmienia jej przy okazji migracji.

`width` zachowuje dotychczasową przybliżoną szerokość emoji i selektorów wariantu. Nie jest kompletnym algorytmem szerokości wszystkich grafemów Unicode. `bar` ogranicza procent do 0–100, szerokość do 0–4096 i używa `█`, `▓`, `░`. `bytes` stosuje jednostki KiB/MiB/GiB, a `size` historyczne etykiety KB/MB/GB/TB raportu. Tekst wejściowy pozostaje danymi — nie jest ponownie parsowany jako NFlow.

Istniejący `progress` w Flow pozostaje właścicielem raportowania postępu, obsługi TTY i cyklu życia spinnera. Ten etap go nie dubluje ani nie zmienia Flow/kernel. Dalsze dopracowanie wyglądu można wykonać tutaj, a do ogólnego Terminal przenieść dopiero sprawdzone, wspólne prymitywy.

Testy: `go test ./bundles/terminal -count=1`. Testy porównawcze srcpack znajdują się w `operators/stage8_*_test.go` i korzystają z wyników wygenerowanych niezależnie na Stage 7.
