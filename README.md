# PostgreSQL-basierte Backend-Implementierung

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)


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


### Architekturentscheidung 
Diese Implementierung stellt eine eigenständig ausführbare Backend-Anwendung mit mandantenbezogener Datenisolierung und relationaler Datenhaltung bereit.

Sie ist als eigenständiges Backend-Projekt mit geringen Infrastrukturabhängigkeiten konzipiert. Die Wahl von SQLite ermöglicht den Betrieb ohne separaten Datenbankserver.

### Zugehöriges Frontend und lokaler Betrieb
Für dieses Backend steht ein kompatibles Frontend auf Basis von React und TypeScript zur Verfügung:
[EventApp Frontend auf GitHub](https://github.com/LindtAna/event-app-frontend)

Beide Komponenten sind aufeinander abgestimmt und können gemeinsam lokal betrieben werden, sodass eine vollständige Event-Management-Anwendung entsteht.
Das Frontend kommuniziert über eine REST-API mit dem Backend. Bei entsprechender Konfiguration der API-Basis-URL und der erforderlichen CORS-Einstellungen können beide Komponenten unabhängig voneinander gestartet werden.


---

## Features
- **Mandantenfähigkeit (Multi-Tenancy):** Tenant-bezogene Datenabfragen und Beziehungen zur Trennung der Daten verschiedener Unternehmen und Organisationen.
- **Benutzerverwaltung:** Registrierung als `owner` (erstellt Tenant / Admin) oder `member` (über Einladungscode). Aktualisierung des Benutzerprofils.
- **Token-basierte Authentifizierung:** Kurzlebiger Access Token (15 Min.) und Refresh Token (7 Tage) in einem HttpOnly-Cookie (mit adaptiven SameSite/Secure-Attributen).
- **Event-Management:** CRUD-Operationen für Events innerhalb des eigenen Tenants; Abfrage nach Ersteller über `ownerId`.
- **Kategorien:** Tenant-bezogene System- und benutzerdefinierte Kategorien. Benutzerdefinierte Kategorien werden bei Bedarf über die Event-Erstellung oder -Aktualisierung angelegt und wieder entfernt, sobald sie nicht mehr verwendet werden.
- **Teilnehmerverwaltung:** Benutzer können sich für Events anmelden und wieder abmelden; ; Teilnehmer eines Events oder Events eines Teilnehmers abrufen.
- Einladungsverwaltung:  Tenant-Owner können Einladungen erstellen, tenantbezogen auflisten, aktive Einladungen widerrufen und Einladungen dauerhaft aus der Datenbank löschen. Die dauerhafte Löschung ist unabhängig vo.m Status der Einladung möglich. Mitglieder registrieren sich mit einem gültigen Einladungscode.
- **Zugriffskontrolle & Rollen:** Einladungverwaltung nur für `owner`; Änderungen und Löschungen von Events nur durch den jeweiligen Ersteller.
- **API-Dokumentation:** Automatisch generierte und interaktive Swagger-UI.
- **CORS:** Middleware zur sicheren Verarbeitung von Cross-Origin-Anfragen basierend auf erlaubten `CLIENT_ORIGIN`-Werten. 

---

## Technologie-Stack
- **Sprache:** Go (Golang)
- **Web-Framework:** [Gin](https://gin-gonic.com/)
- **Datenbank:** PostgreSQL
- **Authentifizierung:** JWT ([`golang-jwt/jwt/v5`](https://github.com/golang-jwt/jwt))
- **Umgebungsvariablen:** [`joho/godotenv`](https://github.com/joho/godotenv)
- **API-Dokumentation:** Swagger / OpenAPI ([`swaggo/gin-swagger`](https://github.com/swaggo/gin-swagger))

---

## Repository-Struktur
```
.
├─ cmd
│  ├── api
│  │   ├── main.go          # Einstiegspunkt der Applikation
│  │   ├── server.go        # HTTP-Server Konfiguration
│  │   ├── routes.go        # Öffentliche und geschützte API-Routen
│  │   ├── middleware.go    # JWT-Authentifizierung, Tenant-Prüfung und CORS
│  │   ├── auth.go          # Registrierung (Owner/Member), Login, Tokens und Profil
│  │   ├── events.go        # Event-Endpunkte
│  │   ├── invitations.go        # Owner-only HTTP-Handler für Einladungen
│  │   ├── categories.go    # HTTP-Handler zum Abrufen der Tenant-Kategorien
│  │   └── context.go       # Helper zum Extrahieren des authentifizierten Benutzers/Tenants
│  └── migrate    # SQL-Up-/Down-Migrationen
│       └── migrations
│           ├── 000001_create_tenants_table.down.sql
│           ├── 000001_create_tenants_table.up.sql
│           ├── 000002_create_users_table.down.sql
│           ├── ...
│           └── 000006_create_invitations_table.up.sql
│ 
├── internal
│   ├── database          # Datenbankverbindung und Models
│   │   ├── tenants.go           # Tenant-Modell
│   │   ├── attendees.go         # Datenbankzugriffe für Event-Teilnehmer (Tenant-geprüft)
│   │   ├── events.go            # Event-Modell, Validierung, CRUD-Operationen
│   │   ├── models.go            # Zusammenfassung der Datenbankmodelle
│   │   ├── categories.go        # Tenantbezogene Kategorie-Modelle
│   │   ├── event_categories.go  # Transaktionsbasierte Kategorie-Logik
│   │   └── invitations.go       # Einladungen-Modell
│   │   └── users.go             # User-Modell
│   └── env                      # Hilfsfunktionen für Umgebungsvariablen
│       └── env.go
├── docs                  # Automatisch generierte Swagger-Dateien
│   └── testing
│        └── api-manual-tests.md  # Anleitung für manuelle API-Tests & cURL-Beispiele
│
├── go.mod    # Go Module Abhängigkeiten
├── go.sum
└── README.md
```
---

## Umgebungsvariablen & Lokaler Start
#### `/EventApp/.env`

```env
PORT=8080
JWT_SECRET=dein-sehr-geheimes-jwt-secret-mit-mindestens-32-bytes
CLIENT_ORIGIN=http://localhost:5173
```
- `PORT` legt den HTTP-Port fest
- `JWT_SECRET` muss mindestens 32 Bytes lang sein (die Anwendung bricht andernfalls beim Start ab)
- `CLIENT_ORIGIN` definiert die erlaubte(n) Frontend-Origin(s)

---
## Ausführen der Migrationen


---

## API-Dokumentation

Die API ist vollständig mit Swagger dokumentiert. Sobald der Server läuft, kann die interaktive Dokumentation im Browser aufgerufen werden unter:

http://localhost:8080/swagger/index.html

Die im Router registrierten API-Routen verwenden das Präfix `/api/v1`.

### API-Endpunkte

#### Authentifizierung und Benutzerprofil

| Methode | Endpunkt | Authentifizierung | Beschreibung |
|---|---|---|---|
| `POST` | `/api/v1/auth/register` | Öffentlich | Tenant-Owner oder Tenant-Member registrieren|
| `POST` | `/api/v1/auth/login` | Öffentlich | Anmelden; Access Token zurückgeben, Refresh-Cookie setzen |
| `POST` | `/api/v1/auth/refresh` | Refresh-Cookie | Neuen Access Token ausstellen |
| `POST` | `/api/v1/auth/logout` | Öffentlich | Refresh-Cookie löschen |
| `PUT` | `/api/v1/users/profile` | Bearer Access Token | Name, Bio und Avatar-URL des eigenen Profils aktualisieren |

#### Registrierungs-Modi (registrationType):
- `owner`: Erstellt einen neuen Tenant und den Administrator. Erfordert `companyName`, `email`, `password` und `name`
- `member`: Tritt einem bestehenden Tenant bei. Übermittelt den erhaltenen `invitationCode` im Feld invitationCode.
Bei der Member-Registrierung prüft der Server, ob die Einladung gültig, nicht abgelaufen, nicht verwendet und nicht widerrufen ist und ob der zugehörige Tenant aktiv ist. Das Anlegen des Mitglieds und das Markieren der Einladung als verwendet erfolgen atomar in einer Transaktion. Tenant-ID, Rolle (member) und Status werden serverseitig festgelegt.
Passwörter müssen zwischen 8 und 72 Zeichen lang sein. E-Mail-Adressen sind global eindeutig und werden ohne Berücksichtigung der Groß-/Kleinschreibung behandelt. Firmennamen werden für die Eindeutigkeitsprüfung normalisiert.

#### Einladungen (Tenant-Owner)

| Methode | Endpunkt | Authentifizierung | Beschreibung |
|---|---|---|---|
| `POST` | `/api/v1/invitations` | Bearer Access Token, Rolle owner | Erstellt eine Einladung für den eigenen Tenant|
| `GET` | `/api/v1/invitations` | Bearer Access Token, Rolle owner | owner	Listet Einladungen des eigenen Tenants aufn |
| `DELETE` | `/api/v1/invitations/:id` | Bearer Access Token, Rolle owner| Widerruft eine aktive Einladung des eigenen Tenants |
| `DELETE` | `/api/v1/invitations/:id/permanent` | Bearer Access Token, Rolle owner| Löscht eine Einladung dauerhaft aus der Datenbank |

`POST /api/v1/invitations` erwartet `expiresAt` als gültigen RFC3339-Zeitstempel in der Zukunft. Die Antwort enthält das Einladungsobjekt und `invitationCode`. Der Klartext-Code wird nur bei der Erstellung ausgegeben; in der Datenbank wird ausschließlich sein SHA-256-Hash gespeichert. Der Code muss daher bei der Erstellung an das eingeladene Mitglied weitergegeben werden.

`DELETE /api/v1/invitations/:id` setzt revoked_at für eine aktive, noch nicht verwendete und nicht abgelaufene Einladung. Bereits verwendete, widerrufene oder abgelaufene Einladungen können über diesen Endpunkt nicht widerrufen werden.

`DELETE /api/v1/invitations/:id/permanent` entfernt den Datensatz vollständig aus der Tabelle invitations. Die Löschung ist unabhängig davon möglich, ob die Einladung aktiv, verwendet, widerrufen oder abgelaufen ist. Der Vorgang ist nicht rückgängig zu machen.

Beide(Widerrufen&Löschen) erfolgreichen Operationen liefern `204 No Content` ohne Response-Body. Eine ungültige ID führt zu `400 Bad Request`.

Alle Endpunkte sind durch die `Access-Token-Middleware` und zusätzlich durch die Owner-Rollenprüfung geschützt. Members erhalten `403 Forbidden`. Die Tenant-Zuordnung wird aus dem authentifizierten Benutzerkontext abgeleitet. Einladungen anderer Tenants werden nicht offengelegt, wird `404 Not Found` zurückgegeben.

#### Registrierungs-Modi (registrationType):
- `owner`: Erstellt einen neuen Tenant und den Administrator. Erfordert companyName, email, password, name
- `member`: Tritt einem bestehenden Tenant bei. Erfordert invitationCode, email, password, name
- Passwörter müssen zwischen 8 und 72 Zeichen lang sein. E-Mail-Adressen und Firmennamen müssen eindeutig sein

#### Registrierungs-Modi (registrationType):
- `owner`: Erstellt einen neuen Tenant und den Administrator. Erfordert companyName, email, password, name
- `member`: Tritt einem bestehenden Tenant bei. Erfordert invitationCode, email, password, name
- Passwörter müssen zwischen 8 und 72 Zeichen lang sein. E-Mail-Adressen und Firmennamen müssen eindeutig sein

#### Events(Tenant-geschützt)

| Methode | Endpunkt | Authentifizierung | Beschreibung |
|---|---|---|---|
| `GET` | `/api/v1/events` | Bearer Access Token | Alle Events des eigenen Tenants abrufen; optionaler Filter ?ownerId=<id> |
| `GET` | `/api/v1/events/:id` | Bearer Access Token | Einzelnes Tenant-Event abrufen |
| `POST` | `/api/v1/events` | Bearer Access Token | Event im eigenen Tenant erstellen |
| `PUT` | `/api/v1/events/:id` | Bearer Access Token | Eigenes Event aktualisieren |
| `DELETE` | `/api/v1/events/:id` | Bearer Access Token | Eigenes Event löschen; erfolgreiche Antwort ist `204 No Content` |

Beim Erstellen wird `ownerId` aus dem authentifizierten Benutzerkontext gesetzt; ein vom Client übermittelter Wert wird ignoriert. Bei Aktualisierung und Löschung muss der authentifizierte Benutzer der Ersteller des Events sein. Ein Event eines anderen Tenants wird als nicht gefunden behandelt.

Der Filter `ownerId` muss eine positive Ganzzahl sein. Ungültige Werte führen zu `400 Bad Request`.

Die Validierung des Event-Modells umfasst unter anderem `title` (3–100 Zeichen), `description` (3–400 Zeichen), `location` (3–200 Zeichen), Start- und Endzeitpunkt sowie eine gültige Kategorie.

#### Kategorien(Tenant-geschützt)

| Methode | Endpunkt | Authentifizierung | Beschreibung |
|---|---|---|---|
| `GET` | `/api/v1/categories` | Bearer Access Token | Alle System- und benutzerdefinierten Kategorien des Tenants abrufen |

Die Tabelle `categories` unterstützt benutzerdefinierte Kategorien (`is_default = 0`). Die Kategorien werden nicht bei der Owner-Registrierung automatisch angelegt; ein neu angelegter Tenant wird deshalb zunächst eine leere Kategorienliste zurückgeben.

Beim Erstellen oder Aktualisieren eines Events kann eine Kategorie  über `categoryName` angegeben werden. Bei `categoryName` sucht die Anwendung innerhalb desselben Tenants nach einer vorhandenen Kategorie. Die Normalisierung entfernt führende und nachgestellte Leerzeichen, fasst aufeinanderfolgende Leerzeichen zusammen und ignoriert Groß-/Kleinschreibung. Falls keine passende Kategorie existiert, wird eine benutzerdefinierte Kategorie angelegt. 
Kategorieauflösung und Event-Änderung erfolgen transaktional. Wenn ein Event geändert oder gelöscht wird, entfernt die Anwendung eine bisherige benutzerdefinierte Kategorie, sofern kein anderes Event sie mehr verwendet. 

#### Teilnehmer(Tenant-geschützt)

| Methode | Endpunkt | Authentifizierung | Beschreibung |
|---|---|---|---|
| `GET` | `/api/v1/events/:id/attendees` | Bearer Access Token | Teilnehmer eines Tenant-Events abrufen |
| `GET` | `/api/v1/attendees/:id/events` | Bearer Access Token | Events abrufen, für die der Benutzer als Teilnehmer eingetragen ist |
| `POST` | `/api/v1/events/:id/attendees/:userId` | Bearer Access Token | Teilnehmer eintragen |
| `DELETE` | `/api/v1/events/:id/attendees/:userId` | Bearer Access Token | Teilnehmer austragen |

Beim Eintragen müssen Event und Zielbenutzer zum authentifizierten Tenant gehören; der Zielbenutzer muss aktiv sein. Eine bereits vorhandene Teilnahme führt zu `409 Conflict`. Eine Teilnahme kann für den eigenen Benutzer angelegt werden. Die Teilnehmerliste enthält Benutzerinformationen aus `id` und `name` und wird nach Name sortiert. Die Events eines Teilnehmers werden nach `start_date_time` aufsteigend sortiert.


### API-Tests



---

## Datenbank-Schema



### tenants
| Feld | Typ | Eigenschaften / Zweck |
|---|---|---|


### users
| Feld | Typ | Eigenschaften / Zweck |
|---|---|---|


### invitations
| Feld | Typ | Eigenschaften / Zweck |
|---|---|---|


### events
| Feld | Typ | Eigenschaften / Zweck |
|---|---|---|


### attendees
| Feld | Typ | Eigenschaften / Zweck |
|---|---|---|


### categories
| Feld | Typ | Eigenschaften / Zweck |
|---|---|---|


### Beziehungen und Löschverhalten




---

## Architektur

Das Projekt folgt einer klaren Separation of Concerns (SoC), trennt HTTP-Verarbeitung, Authentifizierung und Datenbankzugriffe:

- **Routing & Handling (cmd/api):** Registrieren die Endpunkte, validieren Request-Daten, wenden Zugriffskontrollen an und erzeugen HTTP-Antworten.
- **Middleware (`cmd/api/middleware.go`):** Prüft bei geschützten Routen den Bearer Access Token und stellt `user` sowie `userId` im Gin-Kontext bereit. Die CORS-Middleware behandelt Cross-Origin-Anfragen und OPTIONS-Preflight-Requests.
- **Kontext-Helper (`cmd/api/context.go`):** Liest die Identität und Tenant-Zuordnung aus dem authentifizierten Gin-Kontext aus.
- **Datenzugriff (`internal/database`):** Kapselt SQL-Abfragen für Benutzer, Events, Kategorien, Teilnahmen und Einladungen
- Einladungen (`cmd/api/invitations.go`): tellt Owner-only-Endpunkte zum Erstellen, Auflisten, Widerrufen und dauerhaften Löschen von Einladungen bereit.
- **Kategorieauflösung (`internal/database/event_categories.go`):** Löst vorhandene Kategorien auf, erstellt bei Bedarf benutzerdefinierte Kategorien und bereinigt ungenutzte Kategorien in Datenbanktransaktionen.
- **Konfiguration (`internal/env`):** Enthält Hilfsfunktionen zum Laden von Konfigurationswerten.


### Ablauf: Authentifizierung

1. Bei `POST /api/v1/auth/register` wird ein Benutzer gemäß dem Registrierungsmodus `owner` oder `member` angelegt. Passwörter werden gehasht.
2. Bei `POST /api/v1/auth/login` werden E-Mail-Adresse und Passwort geprüft.
3. Bei erfolgreicher Anmeldung wird der Access Token im JSON-Feld `accessToken` zurückgegeben. Der Refresh Token wird im HttpOnly-Cookie `refreshToken` gesetzt.
4. Geschützte Endpunkte erwarten `Authorization: Bearer <accessToken>`.
5. `POST /api/v1/auth/refresh` liest das Refresh-Cookie und gibt einen neuen Access Token sowie das Benutzerobjekt zurück.
6. `POST /api/v1/auth/logout` löscht das Refresh-Cookie.

### Ablauf: Event und Kategorie

Beim Erstellen oder Aktualisieren eines Events werden Event-Änderung und Kategorieauflösung innerhalb einer Datenbanktransaktion zusammengefasst. Eine Kategorie wird entweder anhand einer gültigen `categoryId` gefunden oder anhand von `categoryName` wiederverwendet bzw. neu angelegt. Beim Wechsel oder Löschen eines Events wird geprüft, ob die bisherige benutzerdefinierte Kategorie noch von anderen Events verwendet wird.

### Ablauf: Einladungen

1. Ein Tenant-Owner erstellt über `POST /api/v1/invitations` eine Einladung mit einem zukünftigen Ablaufzeitpunkt.
2. Die API gibt den zufällig generierten `invitationCode` einmalig zurück und speichert nur dessen SHA-256-Hash.
3. Der Owner übermittelt den Code an das einzuladende Mitglied.
4. Das Mitglied registriert sich über `POST /api/v1/auth/register` mit `registrationType: "member"` und `invitationCode`.
5. Die API validiert Einladung und Tenant und erstellt das Mitglied, während die Einladung in derselben Transaktion als verwendet markiert wird.
6. Ein Owner kann Einladungen über `GET /api/v1/invitations` auflisten und aktive, noch verwendbare Einladungen über `DELETE /api/v1/invitations/:id` widerrufen.

### System-Architektur

```text
                          Frontend / HTTP-Client
                                   │
┌───────────────────────────────── ▼ ───────────────────────────────────────┐
│            Gin HTTP-Server                                                │
│             - Gin Logger und Recovery                                     │
│            - CORSMiddleware (global; behandelt auch OPTIONS-Preflight)    │
│                                  |                                        │
│  ┌────────────────────────────── ▼ ─────────────────────────────┐         │
│  │ AuthMiddleware (Prüft Bearer Access Token & Tenant-Status)   │         │
│  └──────────────────────────────────────────────────────────────┘         │
│  ┌────────────────────────│───────────────────────────────────┐           │
│  │                        │                                   │           │
│  │  ┌──────────────────── ▼ ─┐        ┌────────────────────── ▼ ──────┐   │
│  │  │ Öffentliche API-Routen │        │ Geschützte API-Routen         │   │
│  │  │ Präfix: /api/v1        │        │ Präfix: /api/protected        │   │
│  │  │                        │        │                               │   │
│  │  │ Auth                   │        │ Benutzerprofil                │   │
│  │  │- POST /auth/register   │        │- PUT /users/profile           │   │
│  │  │(Modus: owner / member) │        │                               │   │
│  │  │- POST /auth/login      │        │Events                         │   │
│  │  │- POST /auth/refresh    │        │- GET /events                  │   │
│  │  │- POST /auth/logout     │        │- GET /events/:id              │   │
│  │  │                        │        │- POST /events                 │   │
│  │  │                        │        │- PUT /events/:id              │   │
│  │  │                        │        │- DELETE /events/:id           │   │
│  │  │                        │        │                               │   │
│  │  │                        │        │Kategorien                     │   │
│  │  │                        │        │- GET /categories              │   │
│  │  │                        │        │                               │   │
│  │  │                        │        │Teilnahmeverwaltung            │   │
│  │  │                        │        │- GET /events/:id/attendees    │   │
│  │  │                        │        │- GET /attendees/:id/events    │   │
│  │  │                        │        │- POST /events/:id/attendees   │   │
│  │  │                        │        │- DELETE /events/:id/attendees │   │
│  │  │                        │        │                               │   │
│  │  │                        │        │Einladungen (Owner)            │   │
│  │  │                        │        │- POST /invitations            │   │
│  │  │                        │        │- GET /invitations             │   │
│  │  │                        │        │- DELETE /invitations/:id      │   │
│  │  │                        │        │- DELETE /invitations/:id/     │   │
│  │  │                        │        │  /permanent                   │   │
│  │  └────────────────────────┘        └───────────────────────────────┘   │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                               │                                           │
│              ┌─────────────── ▼ ──────────────────┐                       │
│              │ HTTP-Handler / Application Methods │                       │
│              │           (cmd/api)                │                       │
│              └────────────────────────────────────┘                       │
│                              │                                            │
│ ┌─────────────────────────── ▼ ────────────────────────────────────────┐  │
│ │                Datenbankmodelle & Tenant-Logik                       │  │
│ │                    (internal/database)                               │  │
│ │     ┌───────────────────────────────────────────────────────┐        │  │
│ │     │          │        │         │           │             │        │  │
│ │ ┌───▼────┐ ┌───▼──┐ ┌───▼──┐ ┌────▼────┐ ┌────▼──────┐ ┌────▼─────┐  │  │
│ │ │Tenants │ │ Users│ │Events│ │Attendees│ │Invitations│ │Categories│  │  │
│ │ └────────┘ └──────┘ └──────┘ └─────────┘ └───────────┘ └──────────┘  │  │
│ │                        │                                             │  │
│ │       ┌────────────────▼───────────────────┐                         │  │
│ │       │event_categories.go                 │                         │  │
│ │       │- Tenant-Kategorie auflösen         │                         │  │
│ │       │- Transaktional erstellen/bereinigen│                         │  │
│ │       └────────────────────────────────────┘                         │  │
│ └──────────────────────────────────────────────────────────────────────┘  │
│                               │                                           │
│                 ┌─────────────▼─────────────────┐                         │
│                 │                               │                         │
│                 │                               │                         │
│                 └───────────────────────────────┘                         │
│                               │                                           │
│┌───────────────────────────── ▼ ────────────────────────────────────────┐ │
││                                                                        │ │
││                                                                        │ │
││ Tenant-gebundene Tabellen:                                             │ │
││- tenants (id, name, slug, status)                                      │ │
││- users (id, tenant_id, email, name,password, role, status, ...)        │ │
││- events (id, tenant_id, owner_id,title, description, category_id,...)  │ │
││- attendees (id, tenant_id, user_id, event_id, UNIQUE(tenant,event,usr))│ │
││- categories (id, tenant_id, name,is_default)                           │ │
││- invitations (id, tenant_id,token_hash, expires_at, ...)               │ │
│└────────────────────────────────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────────────────────────────┘
Swagger UI: /swagger/*any (außerhalb des Präfixes /api/v1)
```

Die öffentlichen Routen sind nicht durch `AuthMiddleware` geschützt. Für `POST /api/v1/auth/refresh` wird dennoch das `refreshToken`-Cookie benötigt; `POST /api/v1/auth/logout` löscht dieses Cookie. Die geschützten Routen erwarten einen gültigen Bearer Access Token. `CORSMiddleware` wird global registriert. `gin.Default()` aktiviert zusätzlich die Gin-Logger- und Recovery-Middleware.

---

## Sicherheit und Authentifizierung

- **Access Token:** JWT mit HMAC-Signatur; Ablauf nach 15 Minuten.
- **Refresh Token:** JWT mit HMAC-Signatur; Ablauf nach sieben Tagen im HttpOnly-Cookie `refreshToken`. Die Cookie-Attribute werden abhängig von der Verbindung konfiguriert: bei HTTPS `Secure` und `SameSite=None`, lokal über HTTP `SameSite=Lax`.
- **Passwortspeicherung:** Passwörter werden mit bcrypt gehasht und nicht in JSON-Antworten serialisiert. Die Registrierungsvalidierung begrenzt die Passwortlänge auf 8–72 Zeichen; bcrypt verarbeitet höchstens 72 Bytes.
- **Tenant-Isolierung:** Geschützte Handler leiten den Tenant aus dem authentifizierten Benutzerkontext ab. Eine vom Client übermittelte `tenantId` wird nicht als Berechtigungsgrundlage verwendet.
- **Event-Berechtigungen:** Benutzer können Events ihres Tenants abrufen; Aktualisieren und Löschen ist nur dem Ersteller des jeweiligen Events erlaubt.
- **Einladungen:** Verwaltungsendpunkte sind ausschließlich für Tenant-Owner zugänglich. Einladungstoken werden nur bei der Erstellung im Klartext zurückgegeben und in der Datenbank ausschließlich als SHA-256-Hash gespeichert. Einladungen sind einmalig verwendbar und können vor ihrer Verwendung widerrufen werden. Zusätzlich können Tenant-Owner Einladungen unabhängig von ihrem Status dauerhaft aus der Datenbank löschen. Dabei wird die Tenant-Zugehörigkeit anhand des authentifizierten Benutzerkontexts geprüft.
- **CORS:** Leitet Anfragen nur von konfigurierten CLIENT_ORIGIN-Domains durch und erlaubt Credentials.
Diese Cookie- und CORS-Einstellungen werden für eine produktive HTTPS-Bereitstellung passend  konfiguriert.
---

## Roadmap
