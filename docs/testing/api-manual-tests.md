# API-Testanleitung (Multi-Tenant, Authentifizierung & Einladungen)

Die Beispiele sind für Windows `cmd` und eine lokal unter `http://localhost:8080` laufende API ausgelegt.
Die angegebenen HTTP-Statuscodes und Antwortfragmente entsprechen den bereitgestellten Terminal-Ergebnissen. Access-Tokens, Refresh-Cookies und Einladungscodes wurden durch Variablen ersetzt: Werte aus früheren Terminals sind zeitlich begrenzt und dürfen nicht wiederverwendet werden.

## Vorbereitung: Test-Accounts und Tokens

### 1. Owner für Tenant 2 registrieren

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"registrationType\":\"owner\",\"email\":\"owner2@example.com\",\"password\":\"TestPassword123!\",\"name\":\"Owner Test 2\",\"companyName\":\"EventApp Test AG\"}"
```

Korrektes Ergebnis: HTTP/1.1 201 Created 
Die Antwort enthält "id":2 , "tenantId":2 und "role":"owner" 

### 2. Owner 2 anmelden und Access-Token speichern

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" -d "{\"email\":\"owner2@example.com\",\"password\":\"TestPassword123!\"}"
```

Korrektes Ergebnis: HTTP/1.1 200 OK

Die JSON-Antwort enthält accessToken, und der Benutzer hat id: 2, tenantId: 2, role: "owner"

Den vollständigen accessToken aus der Antwort in einer Variable speichern:
```cmd
set "TOKEN2=ACCESS_TOKEN"
```
Den Platzhalter vollständig durch den tatsächlichen Token ersetzen.

### 3. Owner 1 anmelden und Access-Token speichern

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" -d "{\"email\":\"owner1@example.com\",\"password\":\"TestPassword123!\"}"
```
Korrektes Ergebnis: HTTP/1.1 200 OK
Die Antwort enthält accessToken, und der Benutzer hat id:1, tenantId:1, role:"owner".

Den Platzhalter ebenfalls vollständig durch den tatsächlichen Token ersetzen:
```cmd
set "TOKEN1=PASTE_FULL_ACCESS_TOKEN_FROM_RESPONSE"
```

Hinweis: Access-Tokens sind nur begrenzt gültig.
Wenn der Test unerwartet 401 Unauthorized mit Invalid or expired access token zurückgibt, zuerst erneut anmelden und die entsprechende Variable aktualisieren.
(Diese Antwort allein belegt noch keine korrekte Tenant-Isolation.)

---

## A. Registrierung und Eindeutigkeit von Tenant und E-Mail-Adresse

### A1. Zweiten Tenant registrieren
Beim oben beschriebenen Registrierungstest wurde 201 Created zurückgegeben.
Der neue Tenant erhielt eine ID, die sich von der ID von Tenant 1 unterscheidet (tenantId: 2 statt tenantId: 1)

### A2. Doppelte Firmennamen werden nach der Normalisierung erkannt

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"registrationType\":\"owner\",\"email\":\"owner3@example.com\",\"password\":\"TestPassword123!\",\"name\":\"Owner Test 3\",\"companyName\":\" eventapp```test gmbh \"}"
```
Korrektes Ergebnis:HTTP/1.1 409 Conflict 
```json
{"error":"Company name is already in use"}
```
Der bereits registrierte Firmenname EventApp Test GmbH wurde trotz abweichender Groß-/Kleinschreibung, führender und nachgestellter Leerzeichen sowie mehrerer Leerzeichen innerhalb der Zeichenfolge als Duplikat erkannt.

### A3. E-Mail-Adressen müssen global eindeutig und unabhängig von der Groß-/Kleinschreibung sein

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"registrationType\":\"owner\",\"email\":\"OWNER1@EXAMPLE.COM\",\"password\":\"TestPassword123!\",\"name\":\"Owner Test 3\",\"companyName\":\"EventApp Third Test GmbH\"}"
```
Korrektes Ergebnis: HTTP/1.1 409 Conflict 
```json
{"error":"Email address is already registered"}
```


### A4. Anmeldung mit einer E-Mail-Adresse in Großbuchstaben
```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" -d "{\"email\":\"OWNER2@EXAMPLE.COM\",\"password\":\"TestPassword123!\"}"
```
Korrektes Ergebnis: HTTP/1.1 200 OK
Dies bestätigt, dass bei der Anmeldung die Groß-/Kleinschreibung der E-Mail-Adresse nicht berücksichtigt wird
---

## B. Tenant-Isolation: Events und Kategorien

### B1. Leere Event-Liste für Tenant 2

```cmd
curl -i http://localhost:8080/api/v1/events -H "Authorization: Bearer %TOKEN2%"
```
Korrektes Ergebnis: HTTP/1.1 200 OK 
```json
[]
```
### B2. Leere Kategorienliste für Tenant 2
```cmd
curl -i http://localhost:8080/api/v1/categories -H "Authorization: Bearer %TOKEN2%"
```

Korrektes Ergebnis: HTTP/1.1 200 OK 
```json
[]
```

### B3. Event in Tenant 1 erstellen und Kategorie automatisch anlegen

```cmd
curl -i -X POST http://localhost:8080/api/v1/events -H "Authorization: Bearer %TOKEN1%" -H "Content-Type: application/json" -d "{\"title\":\"Tenant A Isolation Test\",\"description\":\"Test event for multi-tenant isolation\",\"location\":\"Cologne Office\",\"startDateTime\":\"2026-11-01T10:00:00Z\",\"endDateTime\":\"2026-11-01T11:00:00Z\",\"categoryName\":\"Isolation Test\"}"
```

Korrektes Ergebnis: HTTP/1.1 201 Created.
Die Antwort enthält ownerId: 1 und categoryId der neu angelegten Kategorie Isolation Test

### B4. Tenant 1 sieht sein eigenes Event

```cmd
curl -i http://localhost:8080/api/v1/events -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis: HTTP/1.1 200 OK 
Die Liste enthält Tenant A Isolation Test mit id: 1 und ownerId: 1

### B5. Tenant 1 sieht seine eigene Kategorie

```cmd
curl -i http://localhost:8080/api/v1/categories -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis: HTTP/1.1 200 OK 
Die Liste enthält Isolation Test mit isDefault:false .


### B6. Tenant 2 sieht weder das Event noch die Kategorie von Tenant 1

```cmd
curl -i http://localhost:8080/api/v1/events -H "Authorization: Bearer %TOKEN2%"
curl -i http://localhost:8080/api/v1/categories -H "Authorization: Bearer %TOKEN2%"
```
Korrektes Ergebnis für beide Anfragen: HTTP/1.1 200 OK и [] .

### B7. Tenant 2 kann das Event von Tenant 1 nicht abrufen

```cmd
curl -i http://localhost:8080/api/v1/events/1 -H "Authorization: Bearer %TOKEN2%"
```
Korrektes Ergebnis: HTTP/1.1 404 Not Found 
```json
{"error":"Event not found"}
```


### B8. Tenant 2 kann das Event von Tenant 1 weder ändern noch löschen
Änderungsversuch (PUT):

```cmd
curl -i -X PUT http://localhost:8080/api/v1/events/1 -H "Authorization: Bearer %TOKEN2%" -H "Content-Type: application/json" -d "{\"title\":\"HACKED BY TENANT B\",\"description\":\"Attempt to modify another tenant event\",\"location\":\"Cologne Office\",\"startDateTime\":\"2026-11-01T10:00:00Z\",\"endDateTime\":\"2026-11-01T11:00:00Z\",\"categoryName\":\"Isolation Test\"}"
```

Löschversuch (DELETE):
```cmd
curl -i -X DELETE http://localhost:8080/api/v1/events/1 -H "Authorization: Bearer %TOKEN2%"
```
Korrektes Ergebnis: 404 Not Found с {"error":"Event not found"}


### B9. Tenant 2 kann beim Erstellen eines Events keine categoryId von Tenant 1 verwenden
Die categoryId aus der Antwort zu B3 kopieren und in den Befehl einsetzen:

```cmd
curl -i -X POST http://localhost:8080/api/v1/events -H "Authorization: Bearer %TOKEN2%" -H "Content-Type: application/json" -d "{\"title\":\"Cross Tenant Category Test\",\"description\":\"Testing tenant isolation\",\"location\":\"Test Office\",\"startDateTime\":\"2026-11-02T10:00:00Z\",\"endDateTime\":\"2026-11-02T11:00:00Z\",\"categoryId\":\"CATEGORY_ID_FROM_TENANT_1\"}"
```
Korrektes Ergebnis: HTTP/1.1 400 Bad Request 
```json
{"error":"Please select a category belonging to your company or provide a category name"}
```

### B10. Eine manipulierte ownerId wird beim Erstellen eines Events ignoriert

```cmd
curl -i -X POST http://localhost:8080/api/v1/events -H "Authorization: Bearer %TOKEN2%" -H "Content-Type: application/json" -d "{\"ownerId\":1,\"title\":\"Owner Spoof Test\",\"description\":\"Testing owner ID protection\",\"location\":\"Test Office\",\"startDateTime\":\"2026-11-02T10:00:00Z\",\"endDateTime\":\"2026-11-02T11:00:00Z\",\"categoryName\":\"Owner Spoof Test\"}"
```
Korrektes Ergebnis: HTTP/1.1 201 Created
Das erstellte Event erhielt id: 2 und ownerId: 2, obwohl in der Anfrage ownerId: 1 übergeben wurde
Die ownerId wird aus dem Kontext des authentifizierten Benutzers abgeleitet

### B11. Tenant 1 kann das Event von Tenant 2 weder abrufen noch ändern oder löschen

```cmd
curl -i http://localhost:8080/api/v1/events/2 -H "Authorization: Bearer %TOKEN1%"
```

```cmd
curl -i -X PUT http://localhost:8080/api/v1/events/2 -H "Authorization: Bearer %TOKEN1%" -H "Content-Type: application/json" -d "{\"title\":\"HACKED BY TENANT A\",\"description\":\"Cross-tenant update attempt\",\"location\":\"Cologne Office\",\"startDateTime\":\"2026-11-02T10:00:00Z\",\"endDateTime\":\"2026-11-02T11:00:00Z\",\"categoryName\":\"Owner Spoof Test\"}"
```

```cmd
curl -i -X DELETE http://localhost:8080/api/v1/events/2 -H "Authorization: Bearer %TOKEN1%"
```

Korrektes Ergebnis für alle drei Anfragen: HTTP/1.1 404 Not Found с {"error":"Event not found"} .

### B12. Filterung nach ownerId unter Berücksichtigung des Tenants

```cmd
curl -i "http://localhost:8080/api/v1/events?ownerId=2" -H "Authorization: Bearer %TOKEN1%"
curl -i "http://localhost:8080/api/v1/events?ownerId=1" -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis: - Bei ownerId=2 erhält Tenant 1 200 OK und [] 
                    - Bei ownerId=1 wird das eigene Event zurückgegeben

Ungültige Filterparameter:

```cmd
curl -i "http://localhost:8080/api/v1/events?ownerId=abc" -H "Authorization: Bearer %TOKEN1%"
curl -i "http://localhost:8080/api/v1/events?ownerId=0" -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis für beide Anfragen: 400 Bad Request 
```json
{"error":"Invalid ownerId parameter"}
```

### B13. Ungültige Event-IDs

```cmd
curl -i http://localhost:8080/api/v1/events/0 -H "Authorization: Bearer %TOKEN1%"
curl -i http://localhost:8080/api/v1/events/abc -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis für beide Anfragen: 400 Bad Request 
```json
{"error":"Invalid event id"}
```

### B14. Eigenes Event ändern und ownerId bei der Aktualisierung schützen

```cmd
curl -i -X PUT http://localhost:8080/api/v1/events/1 -H "Authorization: Bearer %TOKEN1%" -H "Content-Type: application/json" -d "{\"title\":\"Tenant A Isolation Test\",\"description\":\"Update endpoint smoke test\",\"location\":\"Cologne Office\",\"startDateTime\":\"2026-11-01T10:00:00Z\",\"endDateTime\":\"2026-11-01T11:00:00Z\",\"categoryId\":\"CATEGORY_ID_FROM_TENANT_1\"}"
```
Korrektes Ergebnis: HTTP/1.1 200 OK
Die Beschreibung wurde in Update endpoint smoke test geändert.


Anschließend dieselbe Anfrage wiederholen und zusätzlich "ownerId":2 übergeben:
```cmd
curl -i -X PUT http://localhost:8080/api/v1/events/1 -H "Authorization: Bearer %TOKEN1%" -H "Content-Type: application/json" -d "{\"ownerId\":2,\"title\":\"Tenant A Isolation Test\",\"description\":\"Update endpoint smoke test\",\"location\":\"Cologne Office\",\"startDateTime\":\"2026-11-01T10:00:00Z\",\"endDateTime\":\"2026-11-01T11:00:00Z\",\"categoryId\":\"CATEGORY_ID_FROM_TENANT_1\"}"
```
Korrektes Ergebnis: 200 OK; in der Antwort wird weiterhin ownerId: 1 angegeben.

### B15. Bei der Aktualisierung eines Events darf keine fremde Kategorie verwendet werden

Die ID der Kategorie von Tenant 2 (Owner Spoof Test) einsetzen:
```cmd
curl -i -X PUT http://localhost:8080/api/v1/events/1 -H "Authorization: Bearer %TOKEN1%" -H "Content-Type: application/json" -d "{\"title\":\"Tenant A Isolation Test\",\"description\":\"This update must be rejected\",\"location\":\"Cologne Office\",\"startDateTime\":\"2026-11-01T10:00:00Z\",\"endDateTime\":\"2026-11-01T11:00:00Z\",\"categoryId\":\"CATEGORY_ID_FROM_TENANT_2\"}"
```
Korrektes Ergebnis: 400 Bad Request mit der Meldung Please select a category belonging to your company or provide a category name.

Der anschließende GET /api/v1/events/1 zeigte, dass das ursprüngliche Event unverändert blieb.

### B16. Eigenes Event von Tenant 2 löschen

Nach erneuter Anmeldung von Owner 2 einen aktuellen Token verwenden:
```cmd
curl -i http://localhost:8080/api/v1/events/2 -H "Authorization: Bearer %TOKEN2%"
curl -i -X DELETE http://localhost:8080/api/v1/events/2 -H "Authorization: Bearer %TOKEN2%"
curl -i http://localhost:8080/api/v1/events/2 -H "Authorization: Bearer %TOKEN2%"
curl -i -X DELETE http://localhost:8080/api/v1/events/2 -H "Authorization: Bearer %TOKEN2%"
```
Korrektes Ergebnis - erster GET: 200 OK; DELETE - 204 No Content ; 
Der erneute GET-Aufruf und der erneute DELETE-Aufruf liefern::404 Not Found mit Event not found.

Anschließend sollte Tenant 2 für /api/v1/events und /api/v1/categories wieder [] erhalten, während Tenant 1 sein eigenes Event weiterhin sehen sollte.

---

## C. Event-Teilnehmer (attendees)

### C1. Ein anderer Tenant kann weder die Teilnehmerliste eines fremden Events abrufen noch einen Teilnehmer hinzufügen

Mit TOKEN2 (Owner von Tenant 2) und der Event-ID 1 (Tenant 1) testen:
```cmd
curl -i http://localhost:8080/api/v1/events/1/attendees -H "Authorization: Bearer %TOKEN2%"
curl -i -X POST http://localhost:8080/api/v1/events/1/attendees/2 -H "Authorization: Bearer %TOKEN2%"
```
Korrektes Ergebnis: 404 Not Found 
Die Antwort muss Event not found enthalten.

### C2. Teilnehmerliste des eigenen Events abrufen

```cmd
curl -i http://localhost:8080/api/v1/events/1/attendees -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis: 200 OK;
Die Liste war anfangs leer ([]).

### C3. Den eigenen Benutzer zur Teilnehmerliste eines Events hinzufügen

```cmd
curl -i -X POST http://localhost:8080/api/v1/events/1/attendees/1 -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis: 201 Created 
```json
{"id":1,"userId":1,"eventId":1}
```

### C4. Denselben Teilnehmer erneut hinzufügen

```cmd
curl -i -X POST http://localhost:8080/api/v1/events/1/attendees/1 -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis: 409 Conflict 
```json
{"error":"Attendee already exists"}
```

### C5. Teilnehmer entfernen und erneutes Löschen überprüfen

```cmd
curl -i -X DELETE http://localhost:8080/api/v1/events/1/attendees/1 -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis: 204 No Content 

Anschließend ausführen:

```cmd
curl -i http://localhost:8080/api/v1/events/1/attendees -H "Authorization: Bearer %TOKEN1%"
curl -i -X DELETE http://localhost:8080/api/v1/events/1/attendees/1 -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis: 200 OK и []
Korrektes Ergebnis beim erneuten DELETE: 404 Not Found с {"error":"Attendee not found"}


### C6. Events eines Teilnehmers abrufen und die Tenant-Grenze überprüfen

```cmd
curl -i http://localhost:8080/api/v1/attendees/1/events -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis: 200 OK; nach dem Entfernen der Teilnahme war die Liste leer ([])
Nach erneutem Hinzufügen des Teilnehmers wurde in der Antwort das Event von Tenant 1 angezeigt.


### C7. Ein anderer Tenant kann eine Teilnahme nicht löschen

```cmd
curl -i -X DELETE http://localhost:8080/api/v1/events/1/attendees/1 -H "Authorization: Bearer %TOKEN2%"
```
Korrektes Ergebnis: 404 Not Found mit Event not found.

---

## D. Profil aktualisieren und Manipulation von Feldern verhindern

### D1. tenantId, role und status können nicht über die Profilaktualisierung geändert werden

```cmd
curl -i -X PUT http://localhost:8080/api/v1/users/profile -H "Authorization: Bearer %TOKEN2%" -H "Content-Type: application/json" -d "{\"name\":\"Owner Test 2\",\"bio\":\"\",\"avatarUrl\":\"\",\"tenantId\":1,\"role\":\"member\",\"status\":\"active\"}"
```
Korrektes Ergebnis: 200 OK;
Die Werte id: 2 , tenantId: 2 und role: "owner" blieben in der Antwort unverändert

### D2. Das Zielkonto kann nicht durch Übergabe einer anderen id geändert werden

```cmd
curl -i -X PUT http://localhost:8080/api/v1/users/profile -H "Authorization: Bearer %TOKEN2%" -H "Content-Type: application/json" -d "{\"id\":1,\"name\":\"Profile ID Spoof Test\",\"bio\":\"\",\"avatarUrl\":\"\"}"
```
Korrektes Ergebnis: 200 OK
Die Antwort enthielt weiterhin id: 2; der neue Name wurde im Profil von Owner 2 gespeichert.
Die in der Anfrage übergebene ID 1 wurde ignoriert.

Profilnamen nach dem Test wiederherstellen:
```cmd
curl -i -X PUT http://localhost:8080/api/v1/users/profile -H "Authorization: Bearer %TOKEN2%" -H "Content-Type: application/json" -d "{\"name\":\"Owner Test 2\",\"bio\":\"\",\"avatarUrl\":\"\"}"
```
Erwartetes Ergebnis: 200 OK ; der Name lautet wieder Owner Test 2.

---

## E. Authentifizierung und Validierung

### E1. Zugriff auf geschützte Endpunkte ohne Access-Token

```cmd
curl -i http://localhost:8080/api/v1/events
curl -i http://localhost:8080/api/v1/categories
curl -i http://localhost:8080/api/v1/events/1/attendees
curl -i http://localhost:8080/api/v1/attendees/1/events
curl -i -X PUT http://localhost:8080/api/v1/users/profile -H "Content-Type: application/json" -d "{\"name\":\"Anonymous Modification Attempt\",\"bio\":\"\",\"avatarUrl\":\"\"}"
curl -i -X POST http://localhost:8080/api/v1/invitations -H "Content-Type: application/json" -d "{}"
```

Korrektes Ergebnis:  401 Unauthorized
```json
{"error":"A Bearer access token is required"}
```

### E2. Ungültiger Token oder falsches Authentifizierungsschema

```cmd
curl -i http://localhost:8080/api/v1/events -H "Authorization: Bearer invalid-token"
curl -i http://localhost:8080/api/v1/events -H "Authorization: Basic dGVzdDp0ZXN0"
```
Korrektes Ergebnis: 401 Unauthorized
Für Basic Auth: A Bearer access token is required.
Ungültiger oder abgelaufener Bearer-Token: Invalid or expired access token.

### E3. Fehlendes Passwort oder ungültiges JSON bei der Anmeldung

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" -d "{\"email\":\"owner2@example.com\"}"
curl -i -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" -d "{\"email\":\"owner2@example.com\","
```
Korrektes Ergebnis für beide Anfragen: 400 Bad Request 
```json
{"error":"Invalid login data"}
```

### E4. Falsches Passwort oder nicht registrierte E-Mail-Adresse

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" -d "{\"email\":\"owner2@example.com\",\"password\":\"WrongPassword123!\"}"
curl -i -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" -d "{\"email\":\"nobody-404@example.com\",\"password\":\"TestPassword123!\"}"
```
Korrektes Ergebnis für beide Anfragen: 401 Unauthorized 
```json
{"error":"Invalid email or password"}
```

### E5. Owner-Registrierung ohne Firmennamen

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"registrationType\":\"owner\",\"email\":\"owner-no-company@example.com\",\"password\":\"TestPassword123!\",\"name\":\"Owner Without Company\"}"
```
Korrektes Ergebnis: 400 Bad Request 
```json
{"error":"Company name is required for owner registration"}
```

Ein anschließender Anmeldeversuch mit dieser E-Mail-Adresse muss folgendes Ergebnis liefern:
 401 Unauthorized / Invalid email or password, da die Registrierung nicht erfolgreich war.

### E6. Ungültiger Registrierungstyp

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"registrationType\":\"admin\",\"email\":\"owner-invalid-type@example.com\",\"password\":\"TestPassword123!\",\"name\":\"Invalid Registration Type\",\"companyName\":\"EventApp Invalid Type GmbH\"}"
```
Korrektes Ergebnis: 400 Bad Request 
```json
{"error":"Invalid registration data"}
```

### E7. Ungültige E-Mail-Adresse

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"registrationType\":\"owner\",\"email\":\"not-an-email\",\"password\":\"TestPassword123!\",\"name\":\"Invalid Email Test\",\"companyName\":\"EventApp Invalid Email GmbH\"}"
```
Korrektes Ergebnis: 400 Bad Request
```json
{"error":"Invalid registration data"}
```

### E8. Fehlender Name

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"registrationType\":\"owner\",\"email\":\"owner-no-name@example.com\",\"password\":\"TestPassword123!\",\"companyName\":\"EventApp No Name GmbH\"}"
```
Korrektes Ergebnis: 400 Bad Request
```json
{"error":"Invalid registration data"}
```

### E9. Leeres Passwort oder Passwort unterhalb der Mindestlänge

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"registrationType\":\"owner\",\"email\":\"owner-no-password@example.com\",\"password\":\"\",\"name\":\"Owner Without Password\",\"companyName\":\"EventApp No Password GmbH\"}"
curl -i -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"registrationType\":\"owner\",\"email\":\"owner-short-password@example.com\",\"password\":\"123\",\"name\":\"Short Password Test\",\"companyName\":\"EventApp Short Password GmbH\"}"
```
Korrektes Ergebnis: 400 Bad Request
```json
{"error":"Invalid registration data"}
```
Anmeldeversuch mit dem Passwort 123: 400 Bad Request / Invalid login data

### E10. Member-Registrierung ohne Einladung

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"registrationType\":\"member\",\"email\":\"member-no-invite@example.com\",\"password\":\"TestPassword123!\",\"name\":\"Member Without Invite\"}"
```
Korrektes Ergebnis: 400 Bad Request
```json
{"error":"Invitation code is required for member registration"}
```

### E11. Member-Registrierung mit ungültigem Einladungscode

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"registrationType\":\"member\",\"email\":\"member-invalid-invite@example.com\",\"password\":\"TestPassword123!\",\"name\":\"Member Invalid Invite\",\"invitationCode\":\"invalid-test-code\"}"
```
Korrektes Ergebnis: 400 Bad Request
```json
{"error":"Invitation is invalid, expired, used or revoked"}
```
Eine anschließende Anmeldung mit dieser E-Mail-Adresse liefert: 401 Unauthorized / Invalid email or password (das Konto wurde nicht erstellt)

---

## F. Refresh-Token und Abmeldung (logout)

### F1. Owner 2 anmelden und Refresh-Cookie speichern

```cmd
curl -i -c "%TEMP%\eventapp-cookies2.txt" -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" -d "{\"email\":\"owner2@example.com\",\"password\":\"TestPassword123!\"}"
```
Korrektes Ergebnis: 200 OK ; die Antwort enthält Set-Cookie: refreshToken=...; HttpOnly; SameSite=Lax . Das Cookie wird in %TEMP%\eventapp-cookies2.txt gespeichert.

### F2. Access-Token mithilfe des Cookies erneuern

```cmd
curl -i -b "%TEMP%\eventapp-cookies2.txt" -c "%TEMP%\eventapp-cookies2.txt" -X POST http://localhost:8080/api/v1/auth/refresh
```
Korrektes Ergebnis: 200 OK; die Antwort enthält einen neuen accessToken und das Benutzerobjekt.

### F3. Sicherungskopie der Cookie-Datei erstellen und abmelden

```cmd
copy "%TEMP%\eventapp-cookies2.txt" "%TEMP%\eventapp-cookies2-backup.txt"
curl -i -b "%TEMP%\eventapp-cookies2.txt" -c "%TEMP%\eventapp-cookies2.txt" -X POST http://localhost:8080/api/v1/auth/logout
```
Korrektes Ergebnis: 200 OK;
die Antwort lautet {"message":"Successfully logged out"} und das Cookie wird gelöscht ( refreshToken=; Max-Age=0 ).

### F4. Token ohne Cookie erneuern

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/refresh
```
Korrektes Ergebnis: 401 Unauthorized 
```json
{"error":"Refresh token missing"}
```

### F5. Versuch, einen Access-Token als Refresh-Token zu verwenden

```cmd
curl -i -b "refreshToken=%ACCESS_TOKEN%" -X POST http://localhost:8080/api/v1/auth/refresh
```
Zuvor ACCESS_TOKEN setzen und darin den vollständigen Access-Token speichern.
Korrektes Ergebnis: 401 Unauthorized mit {"error":"Invalid refresh token"} ; die Antwort löscht das Cookie.

### F6. Versuch, einen Refresh-Token als Bearer-Access-Token zu verwenden

```cmd
set "REFRESH_TOKEN=PASTE_FULL_REFRESH_TOKEN_FROM_COOKIE_FILE"
curl -i http://localhost:8080/api/v1/events -H "Authorization: Bearer %REFRESH_TOKEN%"
```
Den vollständigen Refresh-Token, der nach der Anmeldung in der Cookie-Datei gespeichert wurde, in die Variable einfügen.
Korrektes Ergebnis: 401 Unauthorized mit Invalid or expired access token. 

---

## G. CORS
### G1. Zulässiger Origin: Preflight-Anfrage

```cmd
curl -i -X OPTIONS http://localhost:8080/api/v1/events -H "Origin: http://localhost:5173" -H "Access-Control-Request-Method: GET" -H "Access-Control-Request-Headers: authorization,content-type"
```
Korrektes Ergebnis: 204 No Content;
insbesondere werden Access-Control-Allow-Origin: http://localhost:5173 und Access-Control-Allow-Credentials: true zurückgegeben.

### G2. Nicht zulässiger Origin: Preflight-Anfrage

```cmd
curl -i -X OPTIONS http://localhost:8080/api/v1/events -H "Origin: https://evil.example" -H "Access-Control-Request-Method: GET" -H "Access-Control-Request-Headers: authorization,content-type"
```
Korrektes Ergebnis: 403 Forbidden 
```json
{"error":"Origin not allowed"}
```

### G3. Nicht zulässiger Origin bei einer regulären Anfrage

```cmd
curl -i http://localhost:8080/api/v1/events -H "Origin: https://evil.example"
```
Korrektes Ergebnis: 403 Forbidden mit {"error":"Origin not allowed"} .

### G4. Zulässiger Origin ohne Authentifizierung

```cmd
curl -i http://localhost:8080/api/v1/events -H "Origin: http://localhost:5173"
```
Korrektes Ergebnis:: 401 Unauthorized
```json
{"error":"A Bearer access token is required"}
```
Dabei sind die erwarteten CORS-Header für http://localhost:5173 vorhanden.

---

## H. API zur Verwaltung von Einladungen

### H1. Einladung erstellen

```cmd
curl -i -X POST http://localhost:8080/api/v1/invitations -H "Authorization: Bearer %TOKEN1%" -H "Content-Type: application/json" -d "{\"expiresAt\":\"2026-10-25T12:00:00+02:00\"}"
```
Korrektes Ergebnis: 201 Created
Die Antwort enthält ein invitation-Objekt mit den Feldern id, tenantId, expiresAt, createdBy und createdAt sowie den nur einmal ausgegebenen invitationCode.

Beispiel für die Struktur der erhaltenen Antwort (die Werte unterscheiden sich bei jedem Aufruf):

```json
{
 "invitation": {
``` "id": 1,
``` "tenantId": 1,
``` "expiresAt": "2026-10-25 10:00:00",
``` "createdBy": 1,
``` "createdAt": "2026-10-22 12:03:48"
 },
 "invitationCode": "PASTE_CODE_FROM_RESPONSE"
}
```
Die angeforderte Uhrzeit 12:00:00+02:00 wurde in der Antwort als 10:00:00 (UTC) zurückgegeben.
Den invitationCode aus der Antwort speichern:
```cmd
set "INVITATION_CODE=invitationCode_FROM_RESPONSE"
```

### H2. Einladungsliste als Owner abrufen

Nach der Anmeldung mit owner1@example.com den gültigen Access-Token in TOKEN1 speichern
```cmd
curl -i http://localhost:8080/api/v1/invitations -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis für eine leere Liste: 200 OK und [].


### H3. Zugriff auf Einladungen ohne Token

```cmd
curl -i -X POST http://localhost:8080/api/v1/invitations -H "Content-Type: application/json" -d "{}"
```
Korrektes Ergebnis: 401 Unauthorized
```json
{"error":"A Bearer access token is required"}
```


### H4. Member mit einer gültigen Einladung registrieren

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"name\":\"Invitation Test Member\",\"email\":\"member.invite.test@example.com\",\"password\":\"TestPassword123!\",\"registrationType\":\"member\",\"invitationCode\":\"%INVITATION_CODE%\"}"
```
Korrektes Ergebnis: 201 Created;
Die Antwort enthält role: "member" und tenantId: 1 (denselben Tenant wie der einladende Owner)

### H5. Registrierten Member anmelden

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" -d "{\"email\":\"member.invite.test@example.com\",\"password\":\"TestPassword123!\"}"
```
Korrektes Ergebnis: 200 OK ;
Die Antwort enthält accessToken und einen Benutzer mit role: "member", tenantId: 1

Für die folgenden Berechtigungstests den Token speichern:
```cmd
set "TOKEN_MEMBER=ACCESS_TOKEN_FROM_RESPONSE"
```

### H6. Ein Member kann Einladungen nicht verwalten

```cmd
curl -i http://localhost:8080/api/v1/invitations -H "Authorization: Bearer %TOKEN_MEMBER%"
```
Korrektes Ergebnis: 403 Forbidden 
```json
{"error":"Only the tenant owner can manage invitations"}
```

### H7. Verwendete Einladungen werden als verwendet markiert

Nach erfolgreicher Registrierung des Members die Liste erneut abrufen:
```cmd
curl -i http://localhost:8080/api/v1/invitations -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis: 200 OK;
Bei verwendeten Einladungen erscheint das Feld usedAt mit einem Zeitstempel.
Bei nicht verwendeten Einladungen ist das Feld usedAt nicht vorhanden.

### H8. Einladung widerrufen
Eine neue Einladung erstellen und ihre ID aus der Antwort notieren.

```cmd
curl -i -X DELETE http://localhost:8080/api/v1/invitations/4 -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis: 204 No Content .

### H9. Registrierung mit einem widerrufenen Einladungscode ablehnen

Den zugehörigen invitationCode vor dem Widerruf speichern.
Anschließend ausführen:

```cmd
curl -i -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"name\":\"Revoked Invitation Test\",\"email\":\"member.revoked.test@example.com\",\"password\":\"TestPassword123!\",\"registrationType\":\"member\",\"invitationCode\":\"%INVITATION_CODE%\"}"
```
Korrektes Ergebnis: 400 Bad Request 
```json
{"error":"Invitation is invalid, expired, used or revoked"}
```

Anschließende Anmeldeversuche mit member.revoked.test@example.com: 401 Unauthorized / Invalid email or password, da das Konto nicht erstellt wurde.


### 10. Einladung löschen

```cmd
curl -i -X DELETE http://localhost:8080/api/v1/invitations/4 -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis: 204 No Content .

---

## I. SQLite: Fremdschlüssel und CASCADE beim Löschen eines Events

### I1. Event mit einem Teilnehmer erstellen

```cmd
curl -i -X POST http://localhost:8080/api/v1/events -H "Authorization: Bearer %TOKEN1%" -H "Content-Type: application/json" -d "{\"title\":\"Tenant A FK Cascade Test\",\"description\":\"Temporary event for SQLite integrity testing\",\"location\":\"Test Office\",\"startDateTime\":\"2026-11-03T10:00:00Z\",\"endDateTime\":\"2026-11-03T11:00:00Z\",\"categoryName\":\"FK Cascade Test\"}"
```
Korrektes Ergebnis: 201 Created, Event-ID 3.

Anschließend die tatsächlich in der Antwort zurückgegebene Event-ID verwenden:

```cmd
curl -i -X POST http://localhost:8080/api/v1/events/3/attendees/1 -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis des durchgeführten Tests: 201 Created mit {"id":3,"userId":1,"eventId":3} 

### I2. Event löschen

```cmd
curl -i -X DELETE http://localhost:8080/api/v1/events/3 -H "Authorization: Bearer %TOKEN1%"
curl -i http://localhost:8080/api/v1/events/3 -H "Authorization: Bearer %TOKEN1%"
```
Korrektes Ergebnis: DELETE — 204 No Content 
Der anschließende GET-Aufruf liefert: 404 Not Found mit Event not found.

### I3. Verbliebene Teilnehmereinträge und Fremdschlüsselverletzungen in SQLite überprüfen

Die SQLite-Datei wurde mit Python schreibgeschützt geöffnet.
Eine entsprechende Prüfung:

```cmd
python -c "import sqlite3; c=sqlite3.connect('file:data.db?mode=ro', uri=True); print('Event 3:', c.execute('SELECT id FROM events WHERE id = 3').fetchall()); print('Attendees for event 3:', c.execute('SELECT id, tenant_id, user_id FROM attendees WHERE event_id = 3').fetchall()); print('Foreign key violations:', c.execute('PRAGMA foreign_key_check').fetchall()); c.close()"
```
Korrektes Ergebnis:
```text
Event 3: []
Attendees for event 3: []
Foreign key violations: []
```

Damit sind weder das Event noch die zugehörigen Teilnehmereinträge in der Datenbank verblieben, und PRAGMA foreign_key_check hat keine Fremdschlüsselverletzungen festgestellt.

---

## Kurze Zusammenfassung der Ergebnisse

- Bestätigt: Tenant 1 und Tenant 2 erhalten getrennte Event- und Kategorienlisten; fremde Events und Benutzer werden mit einer 404-Antwort verborgen
- Bestätigt: Der Owner bzw. Benutzer kann nicht über die Anfragefelder ownerId, tenantId, role, status oder die Profil-ID manipuliert werden
- Bestätigt: Die categoryId eines anderen Tenants wird beim Erstellen oder Ändern eines Events zurückgewiesen
- Bestätigt: Registrierung, Anmeldung, Pflichtfelder, doppelte E-Mail-Adressen, Normalisierung von Firmennamen, Einladungscodes und der ausschließlich auf Owner beschränkte Zugriff auf Einladungen liefern die angegebenen Statuscodes
- Bestätigt: CORS lässt http://localhost:5173 zu und weist https://evil.example zurück
- Bestätigt: Beim Löschen eines Events wurden im Test auch dessen Teilnehmereinträge gelöscht; PRAGMA foreign_key_check lieferte eine leere Liste