# Srcpack Flow CLI demo

`cli_demo.nflow` pokazuje jedną pełną ścieżkę: flagi z `@schema` są montowane jako named pipeline command, `tcli.listen` odkrywa komendę, a request trafia do istniejących operatorów srcpack.

## Co demonstruje

- `PackDemoInput` opisuje wyłącznie opcje wejścia/parsowania, które obecny pipeline wykonuje.
- `@pipeline pack:cli=...:request-schema=PackDemoInput` deklaruje nazwę, help i request schema.
- `srcpack.source -> ast.parse -> stats.collect -> srcpack.render -> collect -> srcpack.join` pozostawia logikę domenową w Go, a kolejność etapów w Flow.
- `tcli.listen` uruchamia pipeline jako komendę `pack`.
- `srcpack.join` skleja wyrenderowane chunki dokładnie w ich kolejności; `tcli` rozpoznaje wynik jako `string` i wypisuje surowy Markdown, bez obudowy JSON/base64.

To celowo **nie jest jeszcze zamiennik pełnego `srcpack` CLI**: `--output`, clipboard, profile/selector grammar, workspace config i wynikowe kody CLI nie są tu realizowane. Osobna schema zawęża powierzchnię zamiast sugerować, że te zachowania już działają.

## Uruchomienie

Plik deklaruje jawnie `@require tcli` i `@require srcpack`. Zwykły `nflow`
rozwiązuje adaptery z modułu źródłowego, jego zależności Go i workspace;
nie wymaga specjalnego hosta ładującego ukryty bundle tcli.
Uruchomienie następuje z katalogu workspace, który ma być pakowany.

Przykładowe argumenty listenera:

```text
pack --target ./cmd --target ./operators --only-sig
pack -t ./cmd -t ./operators --ext go --max-lines 300
pack --file go.mod --file README.md --no-tests
```

## Lokalny test integracyjny

Adapter pochodzi ze zwykłej zależności `github.com/nexssp/transport v0.7.0`.
Nie wymaga lokalnych zmian transportu ani osobnego checkoutu. Testy CLI:

```sh
go test ./cmd/srcpack ./nflows
```
