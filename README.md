# db-sync

Kleines CLI-Tool, um ausgewählte Tabellen von einer Quell-Datenbank in eine Ziel-Datenbank zu prüfen und zu synchronisieren.

Aktuell unterstützt:

- schema analyze ohne Schreibzugriff
- insert-missing für fehlende Zielzeilen
- upsert für bestehende Zielzeilen auf Basis des Primärschlüssels
- optionales mirror-delete für Zielzeilen, die in der Quelle nicht mehr existieren
- MySQL, MariaDB und PostgreSQL als Endpunkte

## Schnellstart

### 1. Lokale Beispiel-Datenbanken starten

```bash
docker compose up -d
```

Das Compose-Setup startet:

- Quelle auf Port 3306
- Ziel auf Port 3307
- Adminer auf Port 8080

Siehe auch [docker-compose.yaml](docker-compose.yaml).

### 2. `.env` anlegen

Als Startpunkt kannst du [.env.example](.env.example) verwenden.

Wichtige Variablen:

```env
DB_SYNC_SOURCE_HOST=localhost
DB_SYNC_SOURCE_PORT=3306
DB_SYNC_SOURCE_USER=dev
DB_SYNC_SOURCE_PASSWORD=dev
DB_SYNC_SOURCE_DB=db
DB_SYNC_SOURCE_ENGINE=mariadb

DB_SYNC_TARGET_HOST=localhost
DB_SYNC_TARGET_PORT=3307
DB_SYNC_TARGET_USER=dev
DB_SYNC_TARGET_PASSWORD=dev
DB_SYNC_TARGET_DB=db
DB_SYNC_TARGET_ENGINE=mariadb

DB_SYNC_TABLES=customer,customer_address,order,order_line_item
DB_SYNC_EXCLUDE_TABLES=app,integration,plugin
DB_SYNC_MIRROR_DELETE=false
```

## CLI verwenden

Ohne Build:

```bash
go run ./cmd/db-sync analyze --env-file .env
go run ./cmd/db-sync run --dry-run --env-file .env
go run ./cmd/db-sync run --env-file .env
```

Mit gebautem Binary:

```bash
go build -o db-sync ./cmd/db-sync
./db-sync analyze --env-file .env
./db-sync run --dry-run --env-file .env
./db-sync version
```

## Releases

Ein Push eines Tags im Format `v*` startet den Release-Workflow in GitHub Actions.
Dabei werden Release-Artefakte fuer Linux, macOS und Windows gebaut und direkt an den GitHub-Release zum Tag angehaengt.

## Commands

### `db-sync analyze`

Prüft die Konfiguration und schreibt nichts ins Ziel.

Die Analyse umfasst:

- Laden der Konfiguration aus den Environment-Variablen
- Verbindungs- und Metadatenprüfung für Source und Target
- Schema-Discovery für die ausgewählten Tabellen
- automatische Einbeziehung benötigter Abhängigkeiten
- getrennte Darstellung von explizit ausgewählten und implizit benötigten Tabellen
- Drift-Report für die final synchronisierten Tabellen

Wenn Blocker gefunden werden, bricht `analyze` mit einem Fehler ab.

### `db-sync run`

Führt die Synchronisierung aus.

Optionen:

- `--dry-run`: zählt geplante Änderungen, schreibt aber nichts ins Ziel

`run` verwendet intern dieselbe Vorprüfung wie `analyze`, gibt bei erfolgreichem Lauf aber nur den Sync-Report aus. Bei `--dry-run` enthält dieser Report die geplanten Inserts, Updates und Deletes.

## Sync-Semantik

- Explizit ausgewählte Tabellen aus `DB_SYNC_TABLES`: fügt fehlende Zeilen ein und aktualisiert bestehende Zielzeilen, wenn sich nicht-PK-Spalten unterscheiden
- Implizit benötigte Relationstabellen: nur fehlende Inserts, keine Updates
- `DB_SYNC_EXCLUDE_TABLES`: schließt Tabellen hart aus, auch wenn sie sonst als implizite Abhängigkeit nachgezogen würden

Bei MySQL und MariaDB deaktiviert `run` die FK-Prüfung auf der gepinnten Target-Session während des Schreiblaufs immer temporär. Das ist nötig, weil reine Insert-Reihenfolge zyklische oder gegenseitig verschachtelte Referenzen zwischen Tabellen nicht generell auflösen kann. Beim Reaktivieren findet keine rückwirkende Validierung bereits geschriebener Zeilen statt.

### `DB_SYNC_MIRROR_DELETE=true`

- Löscht nur in explizit ausgewählten Tabellen Zielzeilen, die in der Quelle nicht mehr vorhanden sind
- Implizit benötigte Relationstabellen werden dabei nicht gelöscht
- Die Löschreihenfolge läuft vor Inserts und Updates rückwärts durch die Abhängigkeitskette der expliziten Tabellen, damit Fremdschlüssel und Unique Keys eher sauber bleiben

## Aktuelle Grenzen

- Der Sync erwartet Primärschlüssel auf den synchronisierten Tabellen
- Abgleich und Updates laufen aktuell zeilenorientiert, nicht chunked
- Upsert basiert aktuell auf Primärschlüsseln, nicht auf separaten Unique Keys
- Es gibt noch keine Rollback-Artefakte oder Checkpoints

## Entwicklung

Relevante Kommandos:

```bash
go test ./...
go test -tags integration ./internal/sync -run TestRunProfileIntegrationUpsertAndMirrorDelete -v
```