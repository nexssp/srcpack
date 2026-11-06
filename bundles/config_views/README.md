# config_views — adapter konfiguracji nad CRUD

`config_views` odpowiada za tożsamość `app`/`schema`, format presetów oraz
zgodność z dawnym `views` srcpack. Wspólne `list/get/save/delete`, konflikt
nazwy, jawne zastępowanie i magazyn YAML należą do [`../crud`](../crud/README.md).
Adapter nie ma własnego kodu odczytu, serializacji ani zapisu plików.

## Zachowane API

```go
_, err := configviews.Execute(ctx, map[string]any{
    "operation": "save", "path": "presets.yml", "name": "review",
    "app": "myapp", "schema": "settings/v1",
    "value": map[string]any{"format": "markdown"},
    "replace": false,
})
```

`operation`: `list`, `get`, `save` lub `delete`; `path` jest wymagane, `name`
wymagane poza `list` (wyjątek legacy poniżej). `replace` domyślnie wynosi
`false`. `app` i `schema` muszą być niepustymi stringami przy `save`; przy
`get/delete` są opcjonalnymi warunkami zgodności. Jawne `replace: true`
zastępuje cały wpis, także jego metadane, bez scalania i historii.

Wyniki pozostają takie jak w Stage 9: `list` zwraca `{names: [...], views: {...}}`,
`get` — `{value: ...}`, `save/delete` — `{name: ...}`. Nazwy są posortowane,
lista zawiera pełne wpisy z metadanymi i nie filtruje po tożsamości.
`errors.Is` rozpoznaje `ErrConflict`, `ErrNotFound` (te same błędy co CRUD)
i `ErrCompatibility`. Nie należy polegać na treści komunikatów błędów CRUD.

```yaml
other_setting: keep
config_views:
  review:
    app: myapp
    schema: settings/v1
    value:
      format: markdown
```

Adapter sprawdza metadane wpisów, ale nie interpretuje schematu samego `value`.
Host odpowiada za walidację treści i ewentualne zastosowanie danych. Dane nie są
wykonywane jako Flow, nie powstają automatyczne snapshoty ani historia rewizji.

## Magazyn jest wymienny

Domyślnie używany jest `crud.YAMLStore`. Można przekazać własny magazyn przez
`crud.WithStore(ctx, store)`, zarówno do `Execute`, jak i bezpośredniej kompozycji
Flow. Kontrakt `Store`, `Snapshot` i `Patch` opisuje README CRUD. Adapter wybiera
sekcję `config_views` albo `views`; magazyn nie zna ich znaczenia.

Magazyn YAML zachowuje niezwiązane pola (także nieznane), może zmienić formatowanie.
Odrzuca niepoprawny YAML, wiele dokumentów, duplikaty kluczy i sekcje inne niż
mapa. Pusty plik i sekcja `null` oznaczają pusty zbiór. Brak pliku nie powoduje
zapisu przy odczycie; `save` może utworzyć plik i katalogi. Zapis atomowy ma tryb
`0600`; nie zapewnia blokady read-modify-write między autorami — host musi ich
serializować. Dokładne ograniczenia serializacji opisuje README CRUD.

## Kompozycja i kopiowanie

Samodzielnie kopiowalnym modułem generycznym jest teraz **CRUD**, a nie adapter.
Aby przenieść również adapter, skopiuj oba katalogi, zachowując sąsiedztwo
`bundles/config_views` i `bundles/crud`, włącznie z plikami `.nflow`, oraz zmień
import CRUD w `config_views/library.go` na ścieżkę swojego modułu. Zależności:
Flow/kernel `v0.35.0`, `gopkg.in/yaml.v3 v3.0.1`, Go `1.26.9`.

Przy ręcznym składaniu hosta Go zamontuj `config_views.FullBundle(nil)` z
`native.Bundles()` — zawiera biblioteki CRUD, więc nie montuj ich drugi raz — i ustaw
`compiler.WithSourceLoader(ctx, config_views.Loader())`:

```nflow
@include "config_views.nflow"
@pipeline my_presets
  @config_views_operation()
@end
pipeline.my_presets
```

Źródło adaptera dołącza `../crud/crud.nflow`. W srcpack loader udostępnia jawnie
`bundles/config_views/config_views.nflow` i `bundles/crud/crud.nflow`, a
NFlow montuje adapter i CRUD osobno przez jawne `@require`.
`config_views.Bundle(nil)` zawiera tylko adapter; `FullBundle(nil)` zachowuje
złożony zestaw dla ręcznych hostów Go. Adapter waliduje kontrakt i wpisy,
wybiera sekcję, opakowuje/rozpakowuje metadane, sprawdza zgodność i deleguje do
`@crud_route()`. Nie powiela implementacji konfliktu, replace ani operacji.
Jedyna akcja Go adaptera tłumaczy błąd wybrany przez politykę NFlow.

## Zgodność starego srcpack

`legacy: true` używa surowych tablic stringów lub `null`, bez metadanych:

```yaml
views:
  review:
    - "src#arch"
    - "internal/auth#source@review"
```

Legacy zachowuje historyczne puste nazwy i `null`. CLI podaje `replace: true`,
by zachować nadpisywanie przez `--save-view`. Normalizacja ścieżek, parsowanie
i rozwijanie selektorów pozostają w `bundles/pack`, nie w CRUD. Rzeczywiste ścieżki
nadal mają pierwszeństwo przed nazwami views, a wartości flag nie są rozwijane.
CLI zachowuje ścisły schemat konfiguracji; do presetów z metadanymi używaj osobnego
pliku, nie dodano pola `config_views` do konfiguracji CLI.

## Testy

`go test ./bundles/config_views -count=1` uruchamia testy Go z `xtest/ktest`:
operacje, metadane, błędy, legacy, inny magazyn bez filesystemu, zachowanie YAML
oraz konsumenta kopiującego adapter razem z CRUD do osobnego modułu bez srcpack.
Niezależność samego CRUD sprawdza osobny zestaw testów w `bundles/crud`.
