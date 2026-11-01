# EventApp Backend – PostgreSQL-basierte Implementierung

![Go](https://img.shields.io/badge/Go-1.26.4-00ADD8?style=flat&logo=go&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/Database-PostgreSQL-4169E1?style=flat&logo=postgresql&logoColor=white)
![Gin](https://img.shields.io/badge/Framework-Gin-008ECF?style=flat)

## Inhaltsverzeichnis

- [Über das Projekt](#uber-das-projekt)
- [Features](#features)
- [Technologie-Stack](#technologie-stack)
- [Repository-Struktur](#repository-struktur)
- [Umgebungsvariablen & Lokaler Start](#umgebungsvariablen--lokaler-start)
- [Ausführen der Migrationen](#ausfuhren-der-migrationen)
- [API-Dokumentation](#api-dokumentation)
  - [API-Endpunkte](#api-endpunkte)
  - [Manuelle API-Tests und cURL-Anleitung](#manuelle-api-tests-und-curl-anleitung)
- [Datenbank-Schema](#datenbank-schema)
  - [Beziehungen und Löschverhalten](#beziehungen-und-loschverhalten)
- [Architektur](#architektur)
  - [Ablauf: Authentifizierung](#ablauf-authentifizierung)
  - [Ablauf: Event und Kategorie](#ablauf-event-und-kategorie)
  - [Ablauf: Einladungen](#ablauf-einladungen)
  - [System-Architektur](#system-architektur)
- [Sicherheit und Authentifizierung](#sicherheit-und-authentifizierung)
- [Roadmap](#roadmap)

## Über das Projekt

EventApp Backend ist eine eigenständig ausführbare REST-API für eine mandantenfähige Event-Management-Anwendung. Die Anwendung ist mit Go und Gin implementiert und verwendet PostgreSQL als relationale Datenbank.

Die Daten verschiedener Unternehmen und Organisationen werden innerhalb einer gemeinsamen Datenbank anhand der Tenant-ID getrennt. Die Tenant-Zugehörigkeit wird serverseitig aus dem authentifizierten Benutzer ermittelt. Zusammengesetzte Fremdschlüssel in der Datenbank unterstützen diese Trennung zusätzlich auf Schemaebene.

### Zugehöriges Frontend

Ein kompatibles Frontend auf Basis von React und TypeScript befindet sich in einem separaten Repository:

[EventApp Frontend auf GitHub](https://github.com/LindtAna/event-app-frontend)

Frontend und Backend kommunizieren über eine REST-API mit dem Präfix `/api/v1`. Beide Anwendungen können unabhängig voneinander gestartet werden. Für den lokalen gemeinsamen Betrieb müssen die API-Basis-URL des Frontends und `CLIENT_ORIGIN` im Backend zueinander passen.


### Architekturentscheidung 
Diese Implementierung stellt eine eigenständig ausführbare Backend-Anwendung mit mandantenbezogener Datenisolierung und relationaler Datenhaltung bereit.


### Zugehöriges Frontend und lokaler Betrieb
Für dieses Backend steht ein kompatibles Frontend auf Basis von React und TypeScript zur Verfügung:
[EventApp Frontend auf GitHub](https://github.com/LindtAna/event-app-frontend)

Beide Komponenten sind aufeinander abgestimmt und können gemeinsam lokal betrieben werden, sodass eine vollständige Event-Management-Anwendung entsteht.
Das Frontend kommuniziert über eine REST-API mit dem Backend. Bei entsprechender Konfiguration der API-Basis-URL und der erforderlichen CORS-Einstellungen können beide Komponenten unabhängig voneinander gestartet werden.

---

## Features

- **Multi-Tenancy:** Tenant-bezogene Abfragen und zusammengesetzte Fremdschlüssel zur Trennung der Daten unterschiedlicher Unternehmen.
- **Benutzerverwaltung:** Registrierung als `owner` mit neuem Tenant oder als `member` mit gültigem Einladungscode; Aktualisierung des eigenen Profils.
- **Authentifizierung:** Kurzlebiger Access Token (15 Minuten) und Refresh Token (7 Tage) als JWT.
- **Event-Management:** Events erstellen, auflisten, einzeln abrufen, aktualisieren und löschen; optionale Filterung nach `ownerId`.
- **Kategorien:** Tenant-bezogene Kategorien. Benutzerdefinierte Kategorien können bei der Event-Erstellung oder -Aktualisierung angelegt und nach ihrer letzten Verwendung automatisch entfernt werden.
- **Teilnehmerverwaltung:** Teilnehmer zu Events hinzufügen und entfernen sowie Teilnehmer eines Events und Events eines Benutzers abrufen.
- **Einladungsverwaltung:** Tenant-Owner können Einladungen erstellen, auflisten, widerrufen und dauerhaft löschen.
- **Rollen- und Zugriffskontrolle:** Owner-only-Zugriff auf die Einladungsverwaltung; Event-Änderungen und -Löschungen sind dem jeweiligen Ersteller vorbehalten.
- **API-Dokumentation:** Interaktive Swagger-UI auf Basis von Swagger / OpenAPI.
- **CORS:** Origin-Allowlist mit Unterstützung für mehrere konfigurierte Frontend-Adressen und Credentials.

---

## Technologie-Stack
- **Sprache:** Go (Golang)
- **HTTP-Framework:** [Gin](https://gin-gonic.com/)
- **Datenbank:** PostgreSQL
- **PostgreSQL-Treiber:** [pgx/v5](https://github.com/jackc/pgx) 5.12.0 über `pgx/stdlib` und `database/sql`
- **Authentifizierung:** JWT ([golang-jwt/jwt/v5](https://github.com/golang-jwt/jwt))
- **Umgebungsvariablen:** [godotenv](https://github.com/joho/godotenv) 1.5.1
- **API-Dokumentation:** Swagger / OpenAPI ([`swaggo/gin-swagger`](https://github.com/swaggo/gin-swagger))

---

## Repository-Struktur
```
.
├─ cmd
│  ├── api
│  │   ├── main.go          # Anwendungseinstieg und PostgreSQL-Verbindung
│  │   ├── server.go        # HTTP-Server Konfiguration
│  │   ├── routes.go        # Öffentliche und geschützte API-Routen
│  │   ├── middleware.go    # JWT-Authentifizierung, Tenant-Prüfung und CORS
│  │   ├── auth.go          # Registrierung (Owner/Member), Login, Tokens und Profil
│  │   ├── events.go        # HTTP-Handler für Events
│  │   ├── invitations.go   # Owner-only-Handler für Einladungen
│  │   ├── categories.go    # HTTP-Handler für Kategorien
│  │   ├── attendees.go     # HTTP-Handler für Event-Teilnehmer
│  │   └── context.go       # Zugriff auf den authentifizierten Benutzer
│  └── migrate    # SQL-Up-/Down-Migrationen
│       ├── main.go         # Separater Einstieg für Datenbankmigrationen
│       └── migrationsс     # SQL-Up- und Down-Migrationen
│           ├── 000001_create_tenants_table.down.sql
│           ├── 000001_create_tenants_table.up.sql
│           ├── 000002_create_users_table.down.sql
│           ├── ...
│           └── 000006_create_invitations_table.up.sql
│ 
├── internal
│   ├── database          # Datenbankverbindung und Models
│   │   ├── models.go            # Initialisierung aller Datenbankmodelle
│   │   ├── tenants.go           # Tenant-Modell und Tenant-Erstellung
│   │   ├── attendees.go         # Teilnehmerzugriffe
│   │   ├── events.go            # Eventabfragen
│   │   ├── categories.go        # Kategorieabfragen
│   │   ├── event_categories.go  # Transaktionale Kategorieauflösung
│   │   └── invitations.go       # Einladungszugriffe
│   │   └── users.go             # Benutzerzugriffe
│   └── env                      # Hilfsfunktionen für Umgebungsvariablen
│       └── env.go
├── docs              # Generierte Swagger-Dateien 
├── .env.example      # Vorlage für lokale Umgebungsvariablen 
├── go.mod    # Go Module Abhängigkeiten
├── go.sum
└── README.md
```
---

## Voraussetzungen

- Go 1.26.4 oder eine kompatible Go-Toolchain, die die im `go.mod` deklarierte Version unterstützt
- Eine erreichbare PostgreSQL-Datenbank
- Für den lokalen Frontend-Betrieb optional das EventApp-Frontend

---

## Umgebungsvariablen
#### `/EventApp/.env`

```env
PORT=8080
JWT_SECRET=replace-this-with-a-random-secret-of-at-least-32-bytes
CLIENT_ORIGIN=http://localhost:5173

# PostgreSQL-Verbindung für die API: gepoolte Verbindung
DATABASE_URL=postgresql://USER:PASSWORD@POOLER_HOST/DATABASE?sslmode=require

# PostgreSQL-Verbindung für Migrationen: direkte, ungepoolte Verbindung
MIGRATIONS_DATABASE_URL=postgresql://USER:PASSWORD@DIRECT_HOST/DATABASE?sslmode=require
```
- `PORT` legt den HTTP-Port fest
- `JWT_SECRET` muss mindestens 32 Bytes lang sein (die Anwendung bricht andernfalls beim Start ab)
- `CLIENT_ORIGIN` definiert die erlaubte(n) Frontend-Origin(s)
- `DATABASE_URL` - PostgreSQL-Verbindungs-URL für die API
- `MIGRATIONS_DATABASE_URL` PostgreSQL-Verbindungs-URL für das Migrationstool(Für Migrationen erforderlich)

Bei Verwendung von Neon sind zwei unterschiedliche Verbindungstypen vorgesehen:

- **`DATABASE_URL`:** gepoolte Verbindungs-URL (Pooled Connection / Pooler) für die regulären Datenbankzugriffe der API.
- **`MIGRATIONS_DATABASE_URL`:** direkte, ungepoolte Verbindungs-URL (Direct / Unpooled Connection) für Schemaänderungen durch `golang-migrate`.

Für beide Variablen werden die jeweils von Neon bereitgestellten Verbindungs-URLs verwendet. Bei einer Pooler-Verbindung enthält der Host üblicherweise den entsprechenden Pooler-Endpunkt; die Migrationsverbindung verwendet den direkten Endpunkt. Zugangsdaten mit reservierten URL-Zeichen müssen URL-kodiert sein.

Die API liest ausschließlich `DATABASE_URL`. Das separate Migrationstool liest ausschließlich `MIGRATIONS_DATABASE_URL`.
---

## Datenbankmigrationen

Die Datenbankmigrationen werden als eigenständiges Kommando ausgeführt. Das Tool verwendet `golang-migrate`, den PostgreSQL-Migrationstreiber und den dateibasierten Source `cmd/migrate/migrations`.

**Wichtig:** Die Befehle müssen aus dem Projektstammverzeichnis gestartet werden, da der Migrations-Source als relativer Pfad angegeben ist.

### Migrationen anwenden
```cmd
go run ./cmd/migrate up
```
Das Tool liest `MIGRATIONS_DATABASE_URL`, prüft zunächst die PostgreSQL-Verbindung und wendet anschließend alle noch ausstehenden Up-Migrationen an. Bereits angewendete Migrationen werden nicht erneut ausgeführt.

### Migrationen zurückrollen

```cmd
go run ./cmd/migrate down
```

`down` führt die verfügbaren Down-Migrationen aus und rollt damit die Migrationen zurück. Dieser Vorgang kann Tabellen und darin gespeicherte Daten entfernen. Verwende ihn daher nur, wenn ein vollständiger Rollback ausdrücklich beabsichtigt ist.

### Reihenfolge der Migrationen

| Version | Datei | Inhalt |
|---|---|---|
| `000001` | `000001_create_tenants_table` | Tenant-Tabelle |
| `000002` | `000002_create_users_table` | Benutzer und Tenant-Zuordnung |
| `000003` | `000003_create_categories_table` | Tenant-bezogene Kategorien |
| `000004` | `000004_create_events_table` | Events und Beziehungen zu Owner und Kategorie |
| `000005` | `000005_create_attendees_table` | Event-Teilnahmen |
| `000006` | `000006_create_invitations_table` | Einladungen und deren Status |

Die Migrationen definieren unter anderem Identity-Spalten für automatisch generierte IDs, Constraints, zusammengesetzte Fremdschlüssel und Indizes. Die Tabellen werden in der oben angegebenen Reihenfolge angelegt, damit die referenzierten Tabellen bereits vorhanden sind.

---

## API-Dokumentation

## API-Dokumentation

Die API verwendet das Präfix `/api/v1`. Swagger wird separat unter `/swagger` bereitgestellt und ist nicht Teil des API-Präfixes.

Bei laufendem lokalem Server:

[Swagger UI öffnen](http://localhost:8080/swagger/index.html)

Geschützte Endpunkte erwarten den Access Token im Header:

```http
Authorization: Bearer <accessToken>
```

#### Authentifizierung und Benutzerprofil
| Methode | Endpunkt | Authentifizierung | Beschreibung |
|---|---|---|---|
| `POST` | `/api/v1/auth/register` | Öffentlich | Tenant-Owner oder eingeladenes Mitglied registrierenregistrieren|
| `POST` | `/api/v1/auth/login` | Öffentlich | Anmelden; Access Token zurückgeben, Refresh-Cookie setzen |
| `POST` | `/api/v1/auth/refresh` | Refresh-Cookie | Neuen Access Token ausstellen |
| `POST` | `/api/v1/auth/logout` | Öffentlich | Refresh-Cookie löschen |
| `PUT` | `/api/v1/users/profile` | Bearer Access Token | Eigenes Profil aktualisieren |

#### Registrierungsmodi:

Das Feld `registrationType` bestimmt den Registrierungsablauf:
- `owner`: Erstellt einen neuen Tenant und den Administrator. Erfordert `companyName`, `email`, `password` und `name`
- `member`: Tritt einem bestehenden Tenant bei. Übermittelt den erhaltenen `invitationCode` im Feld invitationCode.

Bei der Member-Registrierung prüft der Server, ob die Einladung existiert, gültig, nicht abgelaufen, nicht verwendet und nicht widerrufen ist. Außerdem muss der zugehörige Tenant aktiv sein. Das Anlegen des Mitglieds und das Markieren der Einladung als verwendet erfolgen innerhalb derselben Datenbanktransaktion.

Tenant-ID, Rolle (`owner` beziehungsweise `member`) und Status werden serverseitig festgelegt. Vom Client übermittelte Werte sind dafür nicht maßgeblich. Passwörter müssen mindestens 8 Zeichen umfassen und dürfen 72 Bytes nicht überschreiten. E-Mail-Adressen sind global eindeutig und werden ohne Berücksichtigung der Groß-/Kleinschreibung behandelt.

Firmennamen werden vor der Speicherung bereinigt. Für die Eindeutigkeitsprüfung werden Leerzeichen normalisiert und Groß-/Kleinschreibung vereinheitlicht; zusätzlich wird ein eindeutiger Slug erzeugt.

#### Einladungen

Alle Einladungsendpunkte erfordern einen gültigen Bearer Access Token und die Rolle `owner`.

| Methode | Endpunkt | Authentifizierung | Beschreibung |
|---|---|---|---|
| `POST` | `/api/v1/invitations` | Bearer Access Token, Rolle owner | Erstellt eine Einladung für den eigenen Tenant|
| `GET` | `/api/v1/invitations` | Bearer Access Token, Rolle owner | owner	Listet Einladungen des eigenen Tenants aufn |
| `DELETE` | `/api/v1/invitations/:id` | Bearer Access Token, Rolle owner| Widerruft eine aktive Einladung des eigenen Tenants |
| `DELETE` | `/api/v1/invitations/:id/permanent` | Bearer Access Token, Rolle owner| Löscht eine Einladung dauerhaft aus der Datenbank |

`POST /api/v1/invitations` erwartet `expiresAt` als gültigen RFC3339-Zeitstempel in der Zukunft. Die Antwort enthält das Einladungsobjekt und `invitationCode`. Der Klartext-Code wird nur bei der Erstellung ausgegeben; in der Datenbank wird ausschließlich sein SHA-256-Hash gespeichert. Der Code muss daher bei der Erstellung an das eingeladene Mitglied weitergegeben werden.

`DELETE /api/v1/invitations/:id` setzt revoked_at für eine aktive, noch nicht verwendete und nicht abgelaufene Einladung. Bereits verwendete, widerrufene oder abgelaufene Einladungen können über diesen Endpunkt nicht widerrufen werden.

`DELETE /api/v1/invitations/:id/permanent` entfernt den Datensatz vollständig aus der Tabelle invitations. Die Löschung ist unabhängig davon möglich, ob die Einladung aktiv, verwendet, widerrufen oder abgelaufen ist. Der Vorgang ist nicht rückgängig zu machen.

Beide erfolgreichen DELETE-Operationen liefern `204 No Content`. Eine nicht vorhandene oder für den Tenant nicht zugängliche Einladung wird nicht offengelegt und führt zu `404 Not Found`. Die Tenant-Zuordnung wird aus dem authentifizierten Benutzerkontext abgeleitet.


#### Events(Tenant-geschützt)
| Methode | Endpunkt | Authentifizierung | Beschreibung |
|---|---|---|---|
| `GET` | `/api/v1/events` | Bearer Access Token | Alle Events des eigenen Tenants abrufen; optionaler Filter ?ownerId=<id> |
| `GET` | `/api/v1/events/:id` | Bearer Access Token | Einzelnes Tenant-Event abrufen |
| `POST` | `/api/v1/events` | Bearer Access Token | Event im eigenen Tenant erstellen |
| `PUT` | `/api/v1/events/:id` | Bearer Access Token | Eigenes Event aktualisieren |
| `DELETE` | `/api/v1/events/:id` | Bearer Access Token | Eigenes Event löschen |

Beim Erstellen wird `ownerId` aus dem authentifizierten Benutzerkontext gesetzt. Ein vom Client übermittelter Wert wird ignoriert. Bei Aktualisierung und Löschung muss der angemeldete Benutzer der Ersteller des Events sein. Events anderer Tenants werden nicht offengelegt.

Der optionale Filter `ownerId` muss eine positive Ganzzahl sein. Ungültige Werte führen zu `400 Bad Request`. Beim Löschen eines Events antwortet die API mit `204 No Content`.

Die Request-Validierung umfasst unter anderem:

- `title`: 3 bis 100 Zeichen
- `description`: 3 bis 400 Zeichen
- `location`: 3 bis 200 Zeichen
- `startDateTime` und `endDateTime`: erforderlich
- Kategorie: eine gültige `categoryId` oder ein `categoryName`

#### Kategorien

| Methode | Endpunkt | Authentifizierung | Beschreibung |
|---|---|---|---|
| `GET` | `/api/v1/categories` | Bearer Access Token | Kategorien des authentifizierten Tenants abrufen |

Die Tabelle `categories` unterstützt benutzerdefinierte Kategorien (`is_default = 0`). Die Kategorien werden nicht bei der Owner-Registrierung automatisch angelegt; ein neu angelegter Tenant wird deshalb zunächst eine leere Kategorienliste zurückgeben.

Beim Erstellen oder Aktualisieren eines Events kann eine Kategorie  über `categoryName` angegeben werden. Bei `categoryName` sucht die Anwendung innerhalb desselben Tenants nach einer vorhandenen Kategorie. Die Normalisierung entfernt führende und nachgestellte Leerzeichen, fasst aufeinanderfolgende Leerzeichen zusammen und ignoriert Groß-/Kleinschreibung. Falls keine passende Kategorie existiert, wird eine benutzerdefinierte Kategorie angelegt. 
Kategorieauflösung und Event-Änderung erfolgen transaktional. Wenn ein Event geändert oder gelöscht wird, entfernt die Anwendung eine bisherige benutzerdefinierte Kategorie, sofern kein anderes Event sie mehr verwendet. 

#### Teilnehmer
| Methode | Endpunkt | Authentifizierung | Beschreibung |
|---|---|---|---|
| `GET` | `/api/v1/events/:id/attendees` | Bearer Access Token | Teilnehmer eines Tenant-Events auflisten |
| `GET` | `/api/v1/attendees/:id/events` | Bearer Access Token | Events abrufen, für die der Benutzer als Teilnehmer eingetragen ist |
| `POST` | `/api/v1/events/:id/attendees/:userId` | Bearer Access Token | Teilnehmer eintragen |
| `DELETE` | `/api/v1/events/:id/attendees/:userId` | Bearer Access Token | Teilnehmer austragen |

Beim Eintragen müssen Event und Zielbenutzer zum authentifizierten Tenant gehören; der Zielbenutzer muss aktiv sein. Eine bereits vorhandene Teilnahme führt zu `409 Conflict`. Eine Teilnahme kann für den eigenen Benutzer angelegt werden. Die Teilnehmerliste enthält Benutzerinformationen aus `id` und `name` und wird nach Name sortiert. Die Events eines Teilnehmers werden nach `start_date_time` aufsteigend sortiert.

---

## Datenbank-Schema

#### `tenants`
| Spalte | Typ | Eigenschaften / Zweck |
|---|---|---|
| `id` | `INTEGER` | Primärschlüssel, Identity |
| `name` | `TEXT` | Anzeigename des Unternehmens |
| `name_key` | `TEXT` | Normalisierter, eindeutiger Namensschlüssel |
| `slug` | `TEXT` | Eindeutiger URL-tauglicher Bezeichner |
| `status` | `TEXT` | `active` oder `suspended`; Standardwert `active` |
| `created_at` | `TEXT` | Erstellungszeitpunkt, Standardwert in UTC |


#### `users`
| Spalte | Typ | Eigenschaften / Zweck |
|---|---|---|
| `id` | `INTEGER` | Primärschlüssel, Identity |
| `tenant_id` | `INTEGER` | Zugehöriger Tenant, Fremdschlüssel auf `tenants` |
| `email` | `TEXT` | Global eindeutig, ohne Beachtung der Groß-/Kleinschreibung |
| `name` | `TEXT` | Anzeigename |
| `password_hash` | `TEXT` | bcrypt-Passworthash |
| `bio` | `TEXT` | Profilbeschreibung; Standardwert leer |
| `avatar_url` | `TEXT` | Avatar-URL; Standardwert leer |
| `role` | `TEXT` | `owner` oder `member`; Standardwert `member` |
| `status` | `TEXT` | `active` oder `disabled`; Standardwert `active` |
| `created_at` | `TEXT` | Erstellungszeitpunkt, Standardwert in UTC |

Zusätzlich existiert `UNIQUE (tenant_id, id)` als Ziel für zusammengesetzte Fremdschlüssel. Die globale E-Mail-Eindeutigkeit wird über einen eindeutigen Index auf `LOWER(email)` umgesetzt.


#### `invitations`
| Spalte | Typ | Eigenschaften / Zweck |
|---|---|---|
| `id` | `INTEGER` | Primärschlüssel, Identity |
| `tenant_id` | `INTEGER` | Zugehöriger Tenant |
| `token_hash` | `TEXT` | Global eindeutiger SHA-256-Hash des Einladungstokens |
| `expires_at` | `TEXT` | Ablaufzeitpunkt |
| `used_at` | `TEXT` | Zeitpunkt der Verwendung; `NULL`, solange unbenutzt |
| `revoked_at` | `TEXT` | Zeitpunkt des Widerrufs; `NULL`, solange nicht widerrufen |
| `created_by` | `INTEGER` | Erstellender Owner; zusammengesetzter Fremdschlüssel auf `users` |
| `created_at` | `TEXT` | Erstellungszeitpunkt, Standardwert in UTC |


#### `events`
| Spalte | Typ | Eigenschaften / Zweck |
|---|---|---|
| `id` | `INTEGER` | Primärschlüssel, Identity |
| `tenant_id` | `INTEGER` | Zugehöriger Tenant |
| `owner_id` | `INTEGER` | Ersteller; zusammengesetzter Fremdschlüssel auf `users` |
| `title` | `TEXT` | Event-Titel |
| `description` | `TEXT` | Beschreibung |
| `image_url` | `TEXT` | Bild-URL; Standardwert leer |
| `location` | `TEXT` | Veranstaltungsort |
| `start_date_time` | `TEXT` | Beginn des Events |
| `end_date_time` | `TEXT` | Ende des Events |
| `category_id` | `TEXT` | Tenantbezogener Fremdschlüssel auf `categories` |
| `url` | `TEXT` | Externe URL; Standardwert leer |

`UNIQUE (tenant_id, id)` ermöglicht den zusammengesetzten Fremdschlüssel von `attendees`.


#### `attendees`
| Spalte | Typ | Eigenschaften / Zweck |
|---|---|---|
| `id` | `INTEGER` | Primärschlüssel, Identity |
| `tenant_id` | `INTEGER` | Zugehöriger Tenant |
| `user_id` | `INTEGER` | Teilnehmender Benutzer |
| `event_id` | `INTEGER` | Zugehöriges Event |
| `created_at` | `TEXT` | Zeitpunkt der Registrierung |

`UNIQUE (tenant_id, event_id, user_id)` verhindert doppelte Teilnahmen desselben Benutzers an demselben Event innerhalb eines Tenants.


#### `categories`
| Spalte | Typ | Eigenschaften / Zweck |
|---|---|---|
| `tenant_id` | `INTEGER` | Zugehöriger Tenant |
| `id` | `TEXT` | Kategorie-ID; zusammen mit `tenant_id` Primärschlüssel |
| `name` | `TEXT` | Kategoriename |
| `name_key` | `TEXT` | Tenantbezogen eindeutiger, normalisierter Name |
| `is_default` | `INTEGER` | `0` für benutzerdefiniert, `1` für Standardkategorie; CHECK-Constraint |


### Beziehungen und Löschverhalten

| Beziehung | Fremdschlüssel | Löschverhalten |
|---|---|---|
| `users` → `tenants` | `users.tenant_id` → `tenants.id` | `ON DELETE CASCADE` |
| `categories` → `tenants` | `categories.tenant_id` → `tenants.id` | `ON DELETE CASCADE` |
| `events` → `users` | `(tenant_id, owner_id)` → `(tenant_id, id)` | `ON DELETE CASCADE` |
| `events` → `categories` | `(tenant_id, category_id)` → `(tenant_id, id)` | `ON DELETE NO ACTION` |
| `attendees` → `events` | `(tenant_id, event_id)` → `(tenant_id, id)` | `ON DELETE CASCADE` |
| `attendees` → `users` | `(tenant_id, user_id)` → `(tenant_id, id)` | `ON DELETE CASCADE` |
| `invitations` → `users` | `(tenant_id, created_by)` → `(tenant_id, id)` | `ON DELETE CASCADE` |

Die zusammengesetzten Fremdschlüssel stellen sicher, dass verknüpfte Benutzer, Events und Kategorien zum selben Tenant gehören. Ein Event kann daher nicht auf einen Benutzer oder eine Kategorie eines anderen Tenants verweisen.

Die Datenbank-Constraints ergänzen die tenantbezogenen Prüfungen in den Anwendungshandlern; sie ersetzen nicht die Autorisierungsprüfung der API.

---

## Architektur

Das Projekt trennt HTTP-Verarbeitung, Authentifizierung, Datenzugriff und Datenbankmigrationen:

- **HTTP-Server und Routing (`cmd/api`):** Registrieren die Routen, nehmen Requests entgegen und erzeugen HTTP-Antworten.
- **Middleware (`cmd/api/middleware.go`):** Prüft Bearer Access Tokens, lädt den Benutzer erneut aus der Datenbank, überprüft den Status von Benutzer und Tenant und setzt den vertrauenswürdigen Gin-Kontext. Die CORS-Middleware verarbeitet erlaubte Origins und OPTIONS-Preflight-Requests.
- **Kontext- und Handlerlogik (`cmd/api`):** Verwendet die authentifizierte Benutzer- und Tenant-Zuordnung bei den jeweiligen Operationen.
- **Datenzugriff (`internal/database`):** Kapselt SQL-Abfragen und Transaktionen für Tenants, Benutzer, Events, Kategorien, Teilnahmen und Einladungen.
- **PostgreSQL-Verbindung:** `database/sql` wird über `pgx/stdlib` mit PostgreSQL verbunden.
- **Migrationen (`cmd/migrate`):** Ein separates Kommando verwaltet das Datenbankschema über `golang-migrate` und SQL-Dateien.

### Authentifizierungsablauf

1. Bei `POST /api/v1/auth/register` wird ein Owner samt neuem Tenant oder ein eingeladenes Mitglied angelegt. Passwörter werden mit bcrypt gehasht.
2. Bei `POST /api/v1/auth/login` werden E-Mail-Adresse und Passwort geprüft. Benutzer und Tenant müssen aktiv sein.
3. Bei erfolgreicher Anmeldung wird ein kurzlebiger Access Token im JSON-Feld `accessToken` zurückgegeben. Der Refresh Token wird als HttpOnly-Cookie `refreshToken` gesetzt.
4. Geschützte Endpunkte erwarten `Authorization: Bearer <accessToken>`.
5. Die Auth-Middleware lädt den Benutzer bei jedem geschützten Request erneut aus PostgreSQL. Tenant-ID und Rolle werden aus diesem Datenbankdatensatz übernommen, nicht aus Clientdaten.
6. `POST /api/v1/auth/refresh` prüft das Refresh-Cookie und stellt einen neuen Access Token aus.
7. `POST /api/v1/auth/logout` löscht das Refresh-Cookie im Browser.

### Event- und Kategorieablauf

Beim Erstellen oder Aktualisieren eines Events werden Event-Änderung und Kategorieauflösung in einer Datenbanktransaktion ausgeführt. Die Kategorie wird anhand einer gültigen `categoryId` verwendet oder anhand von `categoryName` innerhalb des Tenants gefunden beziehungsweise neu angelegt. Beim Aktualisieren oder Löschen eines Events wird geprüft, ob die bisherige benutzerdefinierte Kategorie noch von einem anderen Event verwendet wird. Nicht mehr verwendete benutzerdefinierte Kategorien werden entfernt.

### Einladungsablauf

1. Ein Tenant-Owner erstellt mit `POST /api/v1/invitations` eine Einladung und legt einen Ablaufzeitpunkt fest.
2. Die API erzeugt einen zufälligen Einladungscode und gibt ihn einmalig zurück. In PostgreSQL wird nur der SHA-256-Hash gespeichert.
3. Der Owner übermittelt den Code an das einzuladende Mitglied.
4. Das Mitglied registriert sich über `POST /api/v1/auth/register` mit `registrationType: "member"` und dem `invitationCode`.
5. Die API prüft Einladung und Tenant. Das Mitglied wird angelegt und die Einladung innerhalb derselben Transaktion als verwendet markiert.
6. Owner können Einladungen auflisten, gültige Einladungen widerrufen und Einladungsdatensätze dauerhaft löschen.

### System-Architektur

```text
                         
                            React / TypeScript Frontend
                                         │
                               REST API: /api/v1
                                         │
                      Bearer Access Token / Refresh Cookie
                                         │
                                         ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                               Go Backend (cmd/api)                                  │
│                                                                                     │
│                                 Gin HTTP-Server                                     │
│                             - Gin Logger und Recovery                               │
│                                                                                     │
│                            CORSMiddleware (global)                                  │
│                             - Origin-Allowlist                                      │
│                             - OPTIONS-Preflight                                     │
│                                         │                                           │
│                                         ▼                                           │
│                                  Routing: /api/v1                                   │
│                                         │                                           │
│              ┌──────────────────────────┴──────────────────────────┐                │
│              │                                                     │                │
│              ▼                                                     ▼                │
│  ┌─────────────────────────────┐                 ┌────────────────────────┐         │
│  │ Öffentliche API-Routen      │                 │ Geschützte API-Routen  │         │
│  │                             │                 │                        │         │
│  │ Authentifizierung           │                 │ AuthMiddleware         │         │
│  │ - POST /auth/register       │                 │ - Bearer Access Token  │         │
│  │ - POST /auth/login          │                 │ - Benutzer aus DB      │         │
│  │ - POST /auth/refresh        │                 │   neu laden            │         │
│  │ - POST /auth/logout         │                 │ - Benutzerstatus prüfen│         │
│  │                             │                 │ - Tenant-Status prüfen │         │
│  └──────────────┬──────────────┘                 └────────────┬───────────┘         │
│                 │                                             │                     │
│                 │                            ┌────────────────┴──────────┐          │
│                 │                            │                           │          │
│                 │                            ▼                           ▼          │
│                 │      ┌──────────────────────────┐  ┌──────────────────────┐       │
│                 │      │ Geschützte Handler       │  │ OwnerOnlyMiddleware  │       │
│                 │      │                          │  │                      │       │
│                 │      │ Benutzerprofil           │  │ Einladungsverwaltung │       │
│                 │      │ Events (CRUD)            │  │ - POST /invitations  │       │
│                 │      │ Kategorien               │  │ - GET /invitations   │       │
│                 │      │ Teilnehmerverwaltung     │  │ - DELETE /:id        │       │
│                 │      │                          │  │ - DELETE /:id/       │       │
│                 │      │                          │  │   permanent          │       │
│                 │      └─────────────┬────────────┘  └──────────┬───────────┘       │
│                 │                    │                          │                   │
│                 └────────────────────┴────┬─────────────────────┘                   │
│                                           │                                         │
│                                           ▼                                         │
│                            HTTP-Handler / Application Methods                       │
│                                       (cmd/api)                                     │
│                                          │                                          │
│                                          ▼                                          │
│  ┌───────────────────────────────────────────────────────────────────────────────┐  │
│  │                      Datenbankmodelle & Tenant-Logik                          │  │
│  │                        (internal/database)                                    │  │
│  │                                                                               │  │
│  │ ┌─────────┐ ┌───────┐ ┌────────┐ ┌───────────┐ ┌────────────┐ ┌─────────────┐ │  │
│  │ │ Tenants │ │ Users │ │ Events │ │ Attendees │ │ Categories │ │ Invitations │ │  │
│  │ └─────────┘ └───────┘ └────────┘ └───────────┘ └────────────┘ └─────────────┘ │  │
│  │                                                                               │  │
│  │   event_categories.go                                                         │  │
│  │   - Kategorieauflösung                                                        │  │
│  │   - Transaktionales Erstellen und Bereinigen                                  │  │
│  │                                                                               │  │
│  │   database/sql + pgx/stdlib                                                   │  │
│  └───────────────────────────────────┬───────────────────────────────────────────┘  │
│                                      │                                              │
│                                 DATABASE_URL                                        │
│                           (Neon Connection Pooler)                                  │
│                                      │                                              │
│                                      ▼                                              │
│                         ┌────────────────────────┐                                  │
│                         │      PostgreSQL        │                                  │
│                         │       (Neon)           │                                  │
│                         │                        │                                  │
│                         │ Tenant-gebundene Daten │                                  │
│                         │ und Beziehungen        │                                  │
│                         └────────────────────────┘                                  │
│                                                                                     │
│  Swagger UI: /swagger/*any (außerhalb von /api/v1)                                  │
└─────────────────────────────────────────────────────────────────────────────────────┘


            Separater Prozess: Datenbankmigrationen
                               │
                               ▼
┌───────────────────────────────────────────────────────────────────┐
│                         cmd/migrate                               │
│                                                                   │
│                        golang-migrate                             │
│                                                                   │
│                    SQL-Up-/Down-Migrationen                       │
│                    cmd/migrate/migrations/                        │
│                                                                   │
│                  - Migrationen anwenden (up)                      │
│                  - Migrationen zurückrollen (down)                │
└───────────────────────────────┬───────────────────────────────────┘
                                │
                    MIGRATIONS_DATABASE_URL
                    (Neon Direct Connection)
                                │
                                ▼
                    ┌──────────────────────┐
                    │      PostgreSQL      │
                    │       (Neon)         │
                    └──────────────────────┘

Swagger UI: /swagger/*any (außerhalb des Präfixes /api/v1)
```

Öffentliche Auth-Routen benötigen keinen Access Token. 
Der Refresh-Endpunkt benötigt ein gültiges `refreshToken`-Cookie.
Die übrigen fachlichen API-Routen sind durch die Auth-Middleware geschützt; die Einladungsverwaltung hat zusätzlich eine Owner-Rollenprüfung.
Swagger UI wird unter `/swagger/*any` bereitgestellt.

---

## Sicherheit

- **JWT-Signatur und Gültigkeit:** Access und Refresh Tokens werden mit HS256 signiert. Der Parser prüft den erwarteten Signaturalgorithmus, Issuer, Ablaufzeit und Token-Typ.
- **Token-Laufzeiten:** Access Token 15 Minuten; Refresh Token 7 Tage.
- **JWT-Schlüssel:** `JWT_SECRET` muss gesetzt sein und mindestens 32 Bytes umfassen.
- **Refresh-Cookie:** Der Refresh Token wird in einem HttpOnly-Cookie mit dem Pfad `/api/v1/auth` gespeichert. Bei HTTPS werden `Secure` und `SameSite=None` gesetzt; bei HTTP wird `SameSite=Lax` verwendet. Es wird keine feste Domain vorgegeben.
- **Passwörter:** Passwörter werden mit bcrypt gehasht und nicht in JSON-Antworten serialisiert.
- **Tenant-Isolierung:** Die Tenant-ID wird aus dem bei jedem geschützten Request erneut geladenen Benutzer abgeleitet. Eine vom Client übermittelte Tenant-ID ist keine Berechtigungsgrundlage.
- **Aktivitätsprüfung:** Geschützte Requests setzen einen aktiven Benutzer und einen aktiven Tenant voraus.
- **Rollen und Event-Berechtigungen:** Nur Owner dürfen Einladungen verwalten. Events dürfen nur von ihrem jeweiligen Ersteller geändert oder gelöscht werden. Bei Teilnehmeroperationen gelten die beschriebenen Rechte des Event-Erstellers und des jeweiligen Benutzers.
- **Einladungstoken:** Der Klartext-Code wird nur bei der Erstellung ausgegeben; gespeichert wird ausschließlich sein SHA-256-Hash. Die Registrierung und das Markieren einer Einladung als verwendet erfolgen atomar.
- **CORS:** Nur konfigurierte `CLIENT_ORIGIN`-Werte werden als Origins zugelassen. Credentials werden für erlaubte Origins unterstützt. 
