# CRUD — niezależne nazwane rekordy

Ten katalog jest kopiowalnym modułem `list/get/save/delete` dowolnych nazwanych
danych serializowalnych do YAML. Nie importuje srcpack, nie zna konfiguracji,
`app`/`schema`, dawnych `views` ani selektorów. Nie jest bazą danych i nie prowadzi
historii rewizji. Wartości są danymi, nigdy kodem do wykonania.

Skopiuj **cały katalog**, włącznie z `crud.nflow`, do własnego modułu. Zależności:
Go `1.26.9`, Flow i kernel `v0.35.0`, `gopkg.in/yaml.v3 v3.0.1`. Nie trzeba zmieniać
wewnętrznych importów CRUD; konsument importuje moduł pod swoją ścieżką.

## Użycie

```go
_, err := crud.Execute(ctx, map[string]any{
    "operation": "save", "path": "data.yml", "name": "review",
    "value": map[string]any{"enabled": true},
})
```

`operation`: `list`, `get`, `save` lub `delete`; `path` jest wymaganym stringiem
(lokalizacją interpretowaną przez magazyn), `section` domyślnie `records`.
`name` musi być niepuste poza `list`, a `replace` jest domyślnie `false`.
`value` może być strukturą, tablicą, skalarem lub `null`.

`list` zwraca `{names: [...], records: {...}}` z posortowanymi nazwami;
`get` zwraca `{value: ...}`, a `save/delete` — `{name: ...}`. Wrapper Flow zwraca
nazwy jako `[]any`. `errors.Is` rozpoznaje `ErrConflict` i `ErrNotFound`.
`save` istniejącej nazwy wymaga `replace: true`; zastępowany jest cały rekord,
bez scalania i bez zachowania poprzedniej wersji. Brak nazwy przy `get/delete`
nie zmienia magazynu.

```yaml
other_setting: keep
records:
  review:
    enabled: true
```

## Polityka i kompozycja

`crud.nflow` deklaruje schematy `CrudOperationInput` i `CrudNamedOperationInput`:
typy, domyślne pola, dozwolone operacje i wymagane pola są opisane raz w `@schema`.
`@crud_operation()` zachowuje warunkowy wymóg nazwy oraz odrzuca wartości złożone
wyłącznie z białych znaków, odczytuje magazyn raz i wywołuje `@crud_route()`.
Adapter może użyć `@crud_route()` po własnej walidacji i odczycie;
nie następuje wtedy drugi odczyt. Bezpośrednie wywołanie route/akcji to interfejs
niskopoziomowy — host musi wcześniej zweryfikować kopertę i rekordy.

Do kompozycji zamontuj `crud.Bundle(nil)` z `native.Bundles()`, ustaw
`compiler.WithSourceLoader(ctx, crud.Loader())`, dołącz `crud.nflow` i wywołaj
`pipeline.crud` lub makro w swoim pipeline. Publiczne `ID` to `crud`, `FS` zawiera
źródło. Bundle montuje wyłącznie akcje `crud.read`, `crud.patch`, `crud.reject`.
Go nie wybiera operacji ani reguł zastępowania; wykonuje magazyn i tłumaczy błędy.

## Wymienny magazyn

```go
type Store interface {
    Read(context.Context, string, string) (Snapshot, error)
    Patch(context.Context, string, string, Snapshot, Patch) error
}
type Snapshot struct {
    Entries map[string]any
    Token   any
}
type Patch struct {
    Key    string
    Remove bool
    Value  any
}
```

Parametry string w obu metodach to `path` i `section`. `Read` zwraca wpisy i token
migawki odczytu. `Patch` mechanicznie zmienia jeden klucz; nie ustala polityki
konfliktu nazwy. `Token` jest przekazywany bez interpretacji. Może służyć własnemu
magazynowi do kontroli współbieżności, ale domyślny magazyn jej nie zapewnia.
Własny magazyn powinien zwracać stabilny widok wpisów i respektować anulowanie.

`crud.WithStore(ctx, store)` wybiera implementację zarówno dla `Execute`, jak i
zewnętrznego runnera Flow, również przez adapter `config_views`. Domyślnie działa
`YAMLStore{}`. Testowy magazyn pamięciowy nie korzysta w ogóle z filesystemu.

Akcja `read` zachowuje wejście, dodając `entries` i `source` (token). Akcja `patch`
przyjmuje `path`, `section`, `source` oraz `patch: {key, remove, value}` i przekazuje
token do `Store.Patch`; `entries` jest opcjonalne. Nie należy wiązać własnego
magazynu z tym, że token magazynu YAML ma postać stringa.

## Bezpieczny YAML, nie transakcje

`YAMLStore` przechowuje w tokenie źródłowy tekst YAML. Zachowuje niezwiązane węzły,
sprawdza serializację i zapisuje atomowo przez `xfs.WriteFileAtomic` z trybem
`0600`. Formatowanie może się zmienić. Brak pliku oznacza pusty zbiór; odczyty go
nie tworzą, a zapis może utworzyć katalogi. Pusty dokument i sekcja `null` są puste.

Niepoprawny YAML, wiele dokumentów, duplikaty kluczy, sekcja inna niż bezpośrednia
mapa lub klucze sekcji inne niż stringi są odrzucane. Aliasy zamiast samej mapy
sekcji nie są obsługiwane. Błędy odczytu, serializacji, anulowania i zapisu są
propagowane; niedopuszczalne wartości Go nie powodują paniki procesu.

Zapis atomowy chroni integralność pliku, **nie izoluje równoległych operacji
read-modify-write**. Host musi serializować autorów lub dostarczyć magazyn z
własną kontrolą współbieżności. Nie ma ukrytych rewizji, automatycznego stosowania
danych ani gwarancji transakcyjnych.

## Testy niezależności

`go test ./bundles/crud -count=1` uruchamia testy Go `_test.go` z `xtest/ktest`:
magazyn pamięciowy, wszystkie operacje, konflikt/replace, null i struktury,
walidację, propagację błędów, zachowanie YAML i awarię zapisu atomowego.
`TestCopiedBundleConsumer` kopiuje produkcję i zestaw testów do osobnego modułu,
uruchamia testy także zewnętrznego konsumenta i sprawdza brak srcpack w zależnościach.
Nie używa asercji shellowych.
