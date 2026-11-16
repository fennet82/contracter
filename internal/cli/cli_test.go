package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// ---- helpers ---------------------------------------------------------------

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// contracter runs the CLI in root and returns its exit code and combined output.
func contracter(root string, args ...string) (int, string) {
	var out bytes.Buffer
	full := append([]string{args[0], "-C", root}, args[1:]...)
	code := Run(full, &out, &out)
	return code, out.String()
}

// repo creates a repo with the given files and contracter.yaml, and snapshots it.
func repo(t *testing.T, config string, files map[string]string, snapshotArgs ...string) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, files)
	write(t, root, map[string]string{"contracter.yaml": config})
	if code, out := contracter(root, append([]string{"snapshot"}, snapshotArgs...)...); code != 0 {
		t.Fatalf("snapshot exit %d:\n%s", code, out)
	}
	return root
}

func contractKeys(t *testing.T, root, entry, file string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "config.contract.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Entries map[string]struct {
			Files map[string]struct {
				Keys map[string]string `json:"keys"`
			} `json:"files"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c.Entries[entry].Files[file].Keys
}

// hasLine reports whether some output line mentions all the given parts.
func hasLine(out string, parts ...string) bool {
next:
	for _, line := range strings.Split(out, "\n") {
		for _, p := range parts {
			if !strings.Contains(line, p) {
				continue next
			}
		}
		return true
	}
	return false
}

// deviation identifies a reported deviation by what tools may rely on. Tests
// assert on the wording shown to people only where that wording is the point.
type deviation struct{ File, Key, Kind string }

// deviations runs a command with -o json and returns what it reported.
func deviations(t *testing.T, root string, args ...string) (int, []deviation) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(append([]string{args[0], "-C", root, "-o", "json"}, args[1:]...), &stdout, &stderr)
	var doc struct{ Deviations []deviation }
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("exit %d, stdout is not the JSON document: %v\n%s%s", code, err, stdout.String(), stderr.String())
	}
	return code, doc.Deviations
}

// wantDeviations runs a command and requires exit 1 and each wanted deviation.
func wantDeviations(t *testing.T, root string, args []string, want ...deviation) {
	t.Helper()
	code, got := deviations(t, root, args...)
	for _, w := range want {
		if code != 1 || !slices.Contains(got, w) {
			t.Fatalf("want exit 1 and %+v, got exit %d and %+v", w, code, got)
		}
	}
}

func wantDeviation(t *testing.T, root, file, key, kind string) {
	t.Helper()
	wantDeviations(t, root, []string{"check"}, deviation{file, key, kind})
}

func wantClean(t *testing.T, root string, args ...string) {
	t.Helper()
	if code, out := contracter(root, append([]string{"check"}, args...)...); code != 0 {
		t.Fatalf("want exit 0, got %d:\n%s", code, out)
	}
}

// ---- formats ---------------------------------------------------------------

func TestSnapshotRecordsKeyTypesForEachFormat(t *testing.T) {
	cases := []struct {
		file, content string
		want          map[string]string
	}{
		{"app.yaml", "db:\n  host: x\n  port: 5432\nservers:\n  - name: a\n    tls: true\nempty: []\n",
			map[string]string{"db.host": "string", "db.port": "number", "servers[].name": "string", "servers[].tls": "bool", "empty": "list"}},
		{"app.json", `{"a":{"b":1.5},"c":null}`,
			map[string]string{"a.b": "number", "c": "null"}},
		{"app.toml", "title = \"x\"\n[db]\nport = 1\n[[srv]]\nname = \"a\"\n",
			map[string]string{"title": "string", "db.port": "number", "srv[].name": "string"}},
		{".env", "export A=1\nB=\"two\"\n",
			map[string]string{"A": "string", "B": "string"}},
		{"app.ini", "top=1\n[db]\nhost=x\n",
			map[string]string{"top": "string", "db.host": "string"}},
		{"app.properties", "db.host=x\nurl=${db.host}/y\n",
			map[string]string{"db.host": "string", "url": "string"}},
		{"settings.xml", `<settings><mirrors><mirror id="a"><url>u1</url></mirror><mirror id="b"><url>u2</url></mirror></mirrors><offline/></settings>`,
			map[string]string{"settings.mirrors.mirror.@id": "string", "settings.mirrors.mirror.url": "string", "settings.offline": "string"}},
		{"main.tf", `
variable "region" {
  type    = string
  default = "us-east-1"
}
module "vpc" {
  source = "terraform-aws-modules/vpc/aws"
  cidr   = var.cidr
  azs    = 3
}
terraform {
  backend "s3" {
    bucket = "dev-state"
  }
}
`,
			map[string]string{
				"variable.region.type": "expression", "variable.region.default": "string",
				"module.vpc.source": "string", "module.vpc.cidr": "expression", "module.vpc.azs": "number",
				"terraform.backend.s3.bucket": "string",
			}},
		{"prod.tfvars", "replicas = 3\n", map[string]string{"replicas": "number"}},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			root := repo(t, "files:\n  - "+c.file+"\n", map[string]string{c.file: c.content})
			if got := contractKeys(t, root, c.file, c.file); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("keys:\n got %v\nwant %v", got, c.want)
			}
			wantClean(t, root)
		})
	}
}

// ---- key deviations --------------------------------------------------------

const appConfig = "files:\n  - path: app.yaml\n    optional: [debug.*]\n    env-specific: [db.host]\n"
const appYAML = "db:\n  host: dev-db.example.com\n  port: 5432\nredis:\n  tls: true\ndebug:\n  trace: false\n"

func TestCheckReportsMissingKey(t *testing.T) {
	root := repo(t, appConfig, map[string]string{"app.yaml": appYAML})
	write(t, root, map[string]string{"app.yaml": "db:\n  host: prod-db\n  port: 5432\ndebug:\n  trace: false\n"})
	wantDeviation(t, root, "app.yaml", "redis.tls", "missing-key")
}

func TestCheckReportsUnknownKey(t *testing.T) {
	root := repo(t, appConfig, map[string]string{"app.yaml": appYAML})
	write(t, root, map[string]string{"app.yaml": strings.Replace(appYAML, "dev-db.example.com", "prod-db", 1) + "stale: 1\n"})
	wantDeviation(t, root, "app.yaml", "stale", "unknown-key")
}

func TestCheckReportsTypeChange(t *testing.T) {
	root := repo(t, appConfig, map[string]string{"app.yaml": appYAML})
	write(t, root, map[string]string{"app.yaml": "db:\n  host: prod-db\n  port: \"5432\"\nredis:\n  tls: true\ndebug:\n  trace: false\n"})
	wantDeviation(t, root, "app.yaml", "db.port", "type-changed")
}

func TestTextOutputHasOneLinePerDeviationAndACount(t *testing.T) {
	root := repo(t, appConfig, map[string]string{"app.yaml": appYAML})
	write(t, root, map[string]string{"app.yaml": "db:\n  host: prod-db\n  port: \"5432\"\ndebug:\n  trace: false\n"})
	code, out := contracter(root, "check")
	if code != 1 || !hasLine(out, "app.yaml", "db.port", "type changed: number -> string") ||
		!hasLine(out, "app.yaml", "redis.tls", "missing key (bool)") || !hasLine(out, "2 deviation(s)") {
		t.Fatalf("got exit %d:\n%s", code, out)
	}
}

func TestCheckReportsEnvSpecificKeyStillHoldingSnapshotValue(t *testing.T) {
	root := repo(t, appConfig, map[string]string{"app.yaml": appYAML})
	wantDeviation(t, root, "app.yaml", "db.host", "value-unchanged")

	write(t, root, map[string]string{"app.yaml": strings.Replace(appYAML, "dev-db.example.com", "prod-db", 1)})
	wantClean(t, root)
}

func TestCheckAllowsOptionalKeysToBeAbsentOrExtra(t *testing.T) {
	root := repo(t, appConfig, map[string]string{"app.yaml": appYAML})
	write(t, root, map[string]string{"app.yaml": "db:\n  host: prod-db\n  port: 5432\nredis:\n  tls: true\ndebug:\n  verbose: true\n"})
	wantClean(t, root)
}

func TestCheckDoesNotCompareListLengthOrEmptiness(t *testing.T) {
	config := "files: [a.yaml]\n"
	root := repo(t, config, map[string]string{"a.yaml": "hosts: []\nports: [80]\nopen: {}\nlater:\n"})
	write(t, root, map[string]string{"a.yaml": "hosts: [{name: a}]\nports: []\nopen: {x: 1}\nlater: {y: 2}\n"})
	wantClean(t, root)
}

func TestCheckReportsMissingFile(t *testing.T) {
	root := repo(t, "files: [a.yaml]\n", map[string]string{"a.yaml": "x: 1\n"})
	os.Remove(filepath.Join(root, "a.yaml"))
	wantDeviation(t, root, "a.yaml", "", "file-missing")
}

func TestCheckReportsUnparseableFile(t *testing.T) {
	root := repo(t, "files: [a.json]\n", map[string]string{"a.json": `{"x": 1}`})
	write(t, root, map[string]string{"a.json": `{"x": `})
	wantDeviation(t, root, "a.json", "", "unreadable")
}

func TestValuePatternMayUseAWildcard(t *testing.T) {
	config := "files:\n  - path: main.tf\n    env-specific: [module.*.source]\n"
	root := repo(t, config, map[string]string{"main.tf": "module \"vpc\" {\n  source = \"registry/vpc\"\n}\n"})
	wantDeviation(t, root, "main.tf", "module.vpc.source", "value-unchanged")
}

// ---- layout ----------------------------------------------------------------

const localesConfig = "files:\n  - public/locales/{lng}/*.json\n"

func locales() map[string]string {
	return map[string]string{
		"public/locales/en/auth.json":    `{"login": {"title": "Sign in"}}`,
		"public/locales/en/billing.json": `{"pay": "Pay"}`,
		"public/locales/he/auth.json":    `{"login": {"title": "כניסה"}}`,
		"public/locales/he/billing.json": `{"pay": "שלם"}`,
	}
}

func TestLayoutReportsFileMissingFromOneInstance(t *testing.T) {
	root := repo(t, localesConfig, locales())
	os.Remove(filepath.Join(root, "public/locales/he/billing.json"))
	wantDeviation(t, root, "public/locales/he/billing.json", "", "file-missing")
}

func TestLayoutReportsKeyMissingFromOneInstance(t *testing.T) {
	root := repo(t, localesConfig, locales())
	write(t, root, map[string]string{"public/locales/he/auth.json": `{"login": {}}`})
	wantDeviation(t, root, "public/locales/he/auth.json", "login.title", "missing-key")
}

func TestLayoutReportsMissingInstance(t *testing.T) {
	root := repo(t, localesConfig, locales())
	os.RemoveAll(filepath.Join(root, "public/locales/he"))
	wantDeviation(t, root, "public/locales/he/auth.json", "", "file-missing")
}

func TestLayoutReportsUnexpectedFile(t *testing.T) {
	root := repo(t, localesConfig, locales())
	write(t, root, map[string]string{"public/locales/en/extra.json": `{}`})
	wantDeviation(t, root, "public/locales/en/extra.json", "", "unexpected-file")
}

func TestLayoutAcceptsExtraInstanceOnlyIfItConforms(t *testing.T) {
	root := repo(t, localesConfig, locales())
	write(t, root, map[string]string{
		"public/locales/fr/auth.json":    `{"login": {"title": "Connexion"}}`,
		"public/locales/fr/billing.json": `{"pay": "Payer"}`,
	})
	wantClean(t, root)

	os.Remove(filepath.Join(root, "public/locales/fr/billing.json"))
	wantDeviation(t, root, "public/locales/fr/billing.json", "", "file-missing")
}

func TestSnapshotFailsWhenInstancesDisagree(t *testing.T) {
	root := t.TempDir()
	files := locales()
	files["public/locales/he/auth.json"] = `{"login": {}}`
	files["contracter.yaml"] = localesConfig
	write(t, root, files)
	wantDeviations(t, root, []string{"snapshot"}, deviation{"public/locales/he/auth.json", "login.title", "missing-key"})
}

// ---- sources ---------------------------------------------------------------

func TestSourceLetsSnapshotReadAConfigThatIsNotARepoFile(t *testing.T) {
	// Where the snapshot is taken, the config lives in a secret manager and arrives as JSON.
	dump := filepath.Join(t.TempDir(), "dump")
	os.WriteFile(dump, []byte(`{"DB_PASS": "dev", "PORT": 8080}`), 0o600)
	config := "files:\n  - path: secrets/app.env\n    env-specific: [DB_PASS]\n"
	root := repo(t, config, nil, "--source", "secrets/app.env=json:"+dump)

	// Where it is checked, the same config is a .env file in the repo. PORT is
	// a string there, and DB_PASS still has the snapshot's value: it passes
	// only because values read through a source are never recorded.
	write(t, root, map[string]string{"secrets/app.env": "DB_PASS=dev\nPORT=8080\n"})
	wantClean(t, root)

	write(t, root, map[string]string{"secrets/app.env": "DB_PASS=x\n"})
	wantDeviation(t, root, "secrets/app.env", "PORT", "missing-key")
}

func TestSourceLetsCheckReadFromOutsideTheRepo(t *testing.T) {
	root := repo(t, localesConfig+"  - app.yaml\n", mergeMaps(locales(), map[string]string{"app.yaml": "a: 1\n"}))
	outside := t.TempDir()
	write(t, outside, map[string]string{
		"loc/en/auth.json":    `{"login": {"title": "x"}}`,
		"loc/en/billing.json": `{}`,
		"app.yaml":            "a: 1\nb: 2\n",
	})
	wantDeviations(t, root,
		[]string{"check",
			"--source", "public/locales=" + filepath.Join(outside, "loc"),
			"--source", "app.yaml=" + filepath.Join(outside, "app.yaml")},
		deviation{"public/locales/en/billing.json", "pay", "missing-key"},
		deviation{"public/locales/he/auth.json", "", "file-missing"},
		deviation{"app.yaml", "b", "unknown-key"})
}

func TestSourceThatMatchesNothingIsAnError(t *testing.T) {
	root := repo(t, "files: [a.yaml]\n", map[string]string{"a.yaml": "x: 1\n"})
	code, out := contracter(root, "check", "--source", "typo.yaml=/somewhere")
	if code != 2 || !strings.Contains(out, "typo.yaml matches no entry") {
		t.Fatalf("want exit 2 naming the source, got %d:\n%s", code, out)
	}
}

func mergeMaps(a, b map[string]string) map[string]string {
	for k, v := range b {
		a[k] = v
	}
	return a
}

// ---- trust boundary --------------------------------------------------------

func TestContractCannotPointOutsideTheRepo(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"config.contract.json": `{"version":1,"entries":{"../secret.yaml":{"files":{"../secret.yaml":{"typed":true,"keys":{}}}}}}`})
	code, out := contracter(root, "check")
	if code != 2 || !strings.Contains(out, "invalid path") {
		t.Fatalf("want exit 2 and 'invalid path', got %d:\n%s", code, out)
	}
}

func TestConfigCannotPointOutsideTheRepo(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"contracter.yaml": "files: [/etc/passwd.yaml]\n"})
	code, out := contracter(root, "snapshot")
	if code != 2 || !strings.Contains(out, "invalid path") {
		t.Fatalf("want exit 2 and 'invalid path', got %d:\n%s", code, out)
	}
}

func TestSnapshotRejectsMisspelledConfigField(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"a.yaml": "x: 1\n", "contracter.yaml": "files:\n  - path: a.yaml\n    env_specific: [x]\n"})
	if code, out := contracter(root, "snapshot"); code != 2 {
		t.Fatalf("want exit 2 for unknown field, got %d:\n%s", code, out)
	}
}

func TestCheckWithoutContractIsAnError(t *testing.T) {
	if code, _ := contracter(t.TempDir(), "check"); code != 2 {
		t.Fatalf("want exit 2, got %d", code)
	}
}

// ---- init ------------------------------------------------------------------

func TestInitListsConfigFilesAndSkipsNonConfig(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"config/app.yaml":              "a: 1\n",
		".env":                         "A=1\n",
		"infra/main.tf":                "",
		"chart/Chart.yaml":             "name: x\n",
		"chart/values.yaml":            "a: 1\n",
		"chart/templates/deploy.yaml":  "{{ .Values.a }}\n",
		"node_modules/pkg/config.json": "{}",
		"package-lock.json":            "{}",
		"src/main.go":                  "package main\n",
	})
	if code, out := contracter(root, "init"); code != 0 {
		t.Fatalf("init exit %d:\n%s", code, out)
	}
	b, _ := os.ReadFile(filepath.Join(root, "contracter.yaml"))
	got := string(b)
	for _, want := range []string{"config/app.yaml", ".env", "infra/main.tf", "chart/values.yaml"} {
		if !hasLine(got, "- "+want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
	for _, not := range []string{"templates", "node_modules", "package-lock", "main.go"} {
		if strings.Contains(got, not) {
			t.Errorf("should not list %s in:\n%s", not, got)
		}
	}
	if code, _ := contracter(root, "snapshot"); code != 0 {
		t.Error("the generated contracter.yaml should be usable by snapshot")
	}
	if code, _ := contracter(root, "init"); code != 2 {
		t.Error("init must refuse to overwrite an existing contracter.yaml")
	}
}

// ---- value checks ----------------------------------------------------------

func TestCheckReportsLockedValueThatChanged(t *testing.T) {
	config := "files:\n  - path: values.yaml\n    locked: [image.tag, features]\n"
	values := "image: {tag: v1.4.2, repo: a}\nfeatures: {beta: false, flags: [a, b]}\n"
	root := repo(t, config, map[string]string{"values.yaml": values})
	wantClean(t, root)

	write(t, root, map[string]string{"values.yaml": strings.Replace(values, "v1.4.2", "v1.4.3", 1)})
	wantDeviation(t, root, "values.yaml", "image.tag", "value-changed")

	write(t, root, map[string]string{"values.yaml": strings.Replace(values, "beta: false", "beta: true", 1)})
	wantDeviation(t, root, "values.yaml", "features.beta", "value-changed")

	write(t, root, map[string]string{"values.yaml": strings.Replace(values, "repo: a", "repo: b", 1)})
	wantClean(t, root) // image.repo is not locked
}

const rulesConfig = `files:
  - path: app.yaml
    rules:
      - key: "*.url"
        match: '^https://'
      - key: image
        not: 'docker\.io'
`

func TestCheckEnforcesValueRules(t *testing.T) {
	// The snapshot source may itself break the rules: they bind the checked copy.
	root := repo(t, rulesConfig, map[string]string{"app.yaml": "api: {url: http://dev}\nimage: docker.io/app\n"})

	code, out := contracter(root, "check")
	if code != 1 || !hasLine(out, "app.yaml", "api.url", "does not match ^https://") || !hasLine(out, "app.yaml", "image", `matches forbidden docker\.io`) {
		t.Fatalf("want both rule violations, got exit %d:\n%s", code, out)
	}
	if strings.Contains(out, "http://dev") || strings.Contains(out, "docker.io/app") {
		t.Fatalf("output must never print config values:\n%s", out)
	}

	write(t, root, map[string]string{"app.yaml": "api: {url: https://prod}\nimage: registry.local/app\n"})
	wantClean(t, root)
}

func TestSnapshotRejectsBrokenRules(t *testing.T) {
	for name, rule := range map[string]string{
		"bad regexp":       "{key: a, match: '('}",
		"no constraint":    "{key: a}",
		"no key":           "{match: x}",
		"misspelled field": "{key: a, macth: x, not: y}",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, map[string]string{"a.yaml": "a: 1\n", "contracter.yaml": "files:\n  - path: a.yaml\n    rules: [" + rule + "]\n"})
			if code, out := contracter(root, "snapshot"); code != 2 {
				t.Fatalf("want exit 2, got %d:\n%s", code, out)
			}
		})
	}
}

func TestKeyPatternStarCrossesAnySeparator(t *testing.T) {
	config := "files:\n  - path: a.yaml\n    optional: ['*.annotations']\n"
	root := repo(t, config, map[string]string{"a.yaml": "svc: {meta: {annotations: {example.com/owner: x}}, port: 1}\n"})
	write(t, root, map[string]string{"a.yaml": "svc: {meta: {annotations: {example.com/team: y}}, port: 1}\n"})
	wantClean(t, root)
}

// ---- snapshot -check -------------------------------------------------------

func TestSnapshotCheckDetectsStaleContractWithoutWriting(t *testing.T) {
	root := repo(t, "files: [a.yaml]\n", map[string]string{"a.yaml": "x: 1\n"})
	if code, out := contracter(root, "snapshot", "-check"); code != 0 {
		t.Fatalf("fresh contract: want exit 0, got %d:\n%s", code, out)
	}
	before, _ := os.ReadFile(filepath.Join(root, "config.contract.json"))

	write(t, root, map[string]string{"a.yaml": "x: 1\ny: 2\n"})
	wantDeviations(t, root, []string{"snapshot", "-check"},
		deviation{"a.yaml", "y", "unknown-key"},
		deviation{"config.contract.json", "", "stale-contract"})
	after, _ := os.ReadFile(filepath.Join(root, "config.contract.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("snapshot -check must not write the contract")
	}
}

func TestSnapshotCheckTreatsChangedConfigAsStale(t *testing.T) {
	root := repo(t, "files: [a.yaml]\n", map[string]string{"a.yaml": "x: 1\n"})
	write(t, root, map[string]string{"contracter.yaml": "files:\n  - path: a.yaml\n    optional: [x]\n"})
	wantDeviations(t, root, []string{"snapshot", "-check"}, deviation{"config.contract.json", "", "stale-contract"})
}

// ---- output ----------------------------------------------------------------

func TestJSONOutputOfACleanCheckIsOnlyAnEmptyList(t *testing.T) {
	root := repo(t, "files: [a.yaml]\n", map[string]string{"a.yaml": "x: 1\n"})
	var stdout, stderr bytes.Buffer
	code := Run([]string{"check", "-C", root, "-o", "json"}, &stdout, &stderr)
	if got := strings.Join(strings.Fields(stdout.String()), ""); code != 0 || got != `{"deviations":[]}` {
		t.Fatalf("exit %d, stdout:\n%s", code, stdout.String())
	}
}

// ---- more layouts ----------------------------------------------------------

func TestPlaceholderInsideAFileNameKeepsSiblingFilesInParity(t *testing.T) {
	root := repo(t, "files:\n  - deploy/values-{env}.yaml\n", map[string]string{
		"deploy/values-dev.yaml":  "replicas: 1\nhost: dev\n",
		"deploy/values-prod.yaml": "replicas: 3\nhost: prod\n",
	})
	wantClean(t, root)
	write(t, root, map[string]string{"deploy/values-prod.yaml": "replicas: 3\n"})
	wantDeviation(t, root, "deploy/values-prod.yaml", "host", "missing-key")
}

func TestMultiDocumentManifestsAreKeyedByKindAndName(t *testing.T) {
	manifests := "kind: Deployment\nmetadata: {name: web}\nspec: {replicas: 2}\n---\nkind: Service\nmetadata: {name: web}\nspec: {port: 80}\n"
	root := repo(t, "files: [k8s.yaml]\n", map[string]string{"k8s.yaml": manifests})
	if got := contractKeys(t, root, "k8s.yaml", "k8s.yaml"); got["Service/web.spec.port"] != "number" || got["Deployment/web.spec.replicas"] != "number" {
		t.Fatalf("keys: %v", got)
	}
	// Dropping a key from one manifest is caught even though another has the same path shape.
	write(t, root, map[string]string{"k8s.yaml": strings.Replace(manifests, "spec: {port: 80}", "spec: {}", 1)})
	wantDeviation(t, root, "k8s.yaml", "Service/web.spec.port", "missing-key")
}

func TestInitCollapsesDirectoriesThatAreAlreadyInParity(t *testing.T) {
	root := t.TempDir()
	write(t, root, mergeMaps(locales(), map[string]string{
		"envs/dev/app.yaml":  "a: 1\n",
		"envs/prod/app.yaml": "a: 1\nonly_prod: 2\n", // not in parity: stays listed per file
	}))
	contracter(root, "init")
	b, _ := os.ReadFile(filepath.Join(root, "contracter.yaml"))
	got := string(b)
	if !hasLine(got, "- public/locales/{name}/*.json") || strings.Contains(got, "locales/en") {
		t.Errorf("locales should collapse into one layout entry:\n%s", got)
	}
	if !hasLine(got, "- envs/dev/app.yaml") || !hasLine(got, "- envs/prod/app.yaml") {
		t.Errorf("directories that disagree must stay listed per file:\n%s", got)
	}
	if code, out := contracter(root, "snapshot"); code != 0 {
		t.Errorf("generated config should snapshot cleanly, got %d:\n%s", code, out)
	}
}

// ---- input that must not break it ------------------------------------------

func TestContractWithNullEntryIsAnError(t *testing.T) {
	for name, contract := range map[string]string{
		"entry": `{"version":1,"entries":{"a.yaml":null}}`,
		"shape": `{"version":1,"entries":{"a.yaml":{"files":{"a.yaml":null}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, map[string]string{"a.yaml": "x: 1\n", "config.contract.json": contract})
			if code, out := contracter(root, "check"); code != 2 || !strings.Contains(out, "null") {
				t.Fatalf("want exit 2 saying what is null, got %d:\n%s", code, out)
			}
		})
	}
}

func TestTrailingYAMLSeparatorDoesNotChangeTheKeys(t *testing.T) {
	root := repo(t, "files: [a.yaml]\n", map[string]string{"a.yaml": "x: 1\n"})
	write(t, root, map[string]string{"a.yaml": "x: 1\n---\n"})
	wantClean(t, root)
}

func TestSnapshotRejectsConfigThatListsNoFiles(t *testing.T) {
	for name, config := range map[string]string{"empty file": "", "empty list": "files: []\n"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, map[string]string{"contracter.yaml": config})
			if code, out := contracter(root, "snapshot"); code != 2 || !strings.Contains(out, "lists no files") {
				t.Fatalf("want exit 2 and 'lists no files', got %d:\n%s", code, out)
			}
		})
	}
}

func TestInitSearchesARootNamedLikeADirectoryItSkips(t *testing.T) {
	root := filepath.Join(t.TempDir(), "build")
	write(t, root, map[string]string{"app.yaml": "x: 1\n", "build/out.yaml": "x: 1\n"})
	contracter(root, "init")
	got, _ := os.ReadFile(filepath.Join(root, "contracter.yaml"))
	if !hasLine(string(got), "- app.yaml") || strings.Contains(string(got), "out.yaml") {
		t.Fatalf("want app.yaml listed and build/out.yaml skipped:\n%s", got)
	}
}

// ---- command line ----------------------------------------------------------

func TestHelpIsNotAnError(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"help"}, {"check", "-h"}} {
		var out bytes.Buffer
		if code := Run(args, &out, &out); code != 0 || !strings.Contains(out.String(), "usage: contracter") {
			t.Errorf("%v: want exit 0 and the usage, got %d:\n%s", args, code, out.String())
		}
	}
}

func TestCommandRejectsAFlagItDoesNotHave(t *testing.T) {
	root := repo(t, "files: [a.yaml]\n", map[string]string{"a.yaml": "x: 1\n"})
	if code, _ := contracter(root, "check", "-check"); code != 2 {
		t.Fatalf("check has no -check flag: want exit 2, got %d", code)
	}
}

func TestSnapshotCheckDoesNotSayOKWhenInstancesDisagree(t *testing.T) {
	files := locales()
	files["public/locales/he/auth.json"] = `{"login": {}}`
	files["contracter.yaml"] = localesConfig
	root := t.TempDir()
	write(t, root, files)
	contracter(root, "snapshot")
	if code, out := contracter(root, "snapshot", "-check"); code != 1 || strings.Contains(out, "ok:") {
		t.Fatalf("want exit 1 and no ok line, got %d:\n%s", code, out)
	}
}

func TestInitUnderAnotherNameDoesNotListContractersOwnFiles(t *testing.T) {
	root := repo(t, "files: [app.yaml]\n", map[string]string{"app.yaml": "x: 1\n", "new.yaml": "y: 1\n"})
	contracter(root, "init", "-config", "detected.yaml")
	got, _ := os.ReadFile(filepath.Join(root, "detected.yaml"))
	if !hasLine(string(got), "- new.yaml") || hasLine(string(got), "- contracter.yaml") || hasLine(string(got), "- config.contract.json") {
		t.Fatalf("want new.yaml listed and no contracter file:\n%s", got)
	}
}

// ---- what init leaves out ---------------------------------------------------

func TestInitAppliesTheReposIgnoreFileOnTopOfTheDefaults(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"app.yaml":                 "x: 1\n",
		"tests/fixtures/data.json": "{}",
		"docs/openapi.yaml":        "x: 1\n",
		"docs/keep.json":           "{}",
		"vendor/chart/values.yaml": "x: 1\n", // ignored by default, brought back below
		"node_modules/p/c.json":    "{}",     // ignored by default, stays ignored
		".contracterignore":        "# ours\nfixtures/\ndocs/*.yaml\n!vendor/\n",
	})
	if code, out := contracter(root, "init"); code != 0 {
		t.Fatalf("init exit %d:\n%s", code, out)
	}
	b, _ := os.ReadFile(filepath.Join(root, "contracter.yaml"))
	got := string(b)
	for _, want := range []string{"app.yaml", "docs/keep.json", "vendor/chart/values.yaml"} {
		if !hasLine(got, "- "+want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
	for _, not := range []string{"fixtures", "openapi", "node_modules"} {
		if strings.Contains(got, not) {
			t.Errorf("should not list %s in:\n%s", not, got)
		}
	}
}

func TestInitRejectsAnIgnoreLineThatCannotWork(t *testing.T) {
	for name, line := range map[string]string{"malformed glob": "[", "comment after a pattern": "fixtures/ # test data"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, map[string]string{"app.yaml": "x: 1\n", ".contracterignore": "ok\n" + line + "\n"})
			if code, out := contracter(root, "init"); code != 2 || !strings.Contains(out, ".contracterignore: line 2") {
				t.Fatalf("want exit 2 naming the file and line, got %d:\n%s", code, out)
			}
		})
	}
}

func TestInitWritesNothingWhenItFindsNoConfig(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"main.go": "package main\n"})
	code, out := contracter(root, "init")
	if _, err := os.Stat(filepath.Join(root, "contracter.yaml")); code != 2 || err == nil {
		t.Fatalf("want exit 2 and no contracter.yaml, got %d (stat err %v):\n%s", code, err, out)
	}
}

func TestSourceDirectoryMayHoldSymlinkedFiles(t *testing.T) {
	// Mounted config (a Kubernetes ConfigMap volume, for one) is made of links.
	root := repo(t, "files: ['conf/*.yaml']\n", map[string]string{"conf/app.yaml": "x: 1\n"})
	mount := t.TempDir()
	write(t, mount, map[string]string{".data/app.yaml": "x: 2\n"})
	if err := os.Symlink(filepath.Join(mount, ".data/app.yaml"), filepath.Join(mount, "app.yaml")); err != nil {
		t.Skip("cannot create symlinks here:", err)
	}
	wantClean(t, root, "-source", "conf="+mount)
}

func TestRepoSymlinksAreNotFollowed(t *testing.T) {
	// A repo is input: a link in it must not make contracter read elsewhere.
	outside := t.TempDir()
	write(t, outside, map[string]string{"secret.yaml": "password: x\n"})
	root := repo(t, "files: ['conf/*.yaml']\n", map[string]string{"conf/app.yaml": "x: 1\n"})
	if err := os.Symlink(filepath.Join(outside, "secret.yaml"), filepath.Join(root, "conf/linked.yaml")); err != nil {
		t.Skip("cannot create symlinks here:", err)
	}
	if code, out := contracter(root, "check"); code != 0 || strings.Contains(out, "password") {
		t.Fatalf("the link must be ignored, got exit %d:\n%s", code, out)
	}
}

func TestUnknownOutputFormatIsAnError(t *testing.T) {
	root := repo(t, "files: [a.yaml]\n", map[string]string{"a.yaml": "x: 1\n"})
	if code, out := contracter(root, "check", "-o", "xml"); code != 2 || !strings.Contains(out, "xml") {
		t.Fatalf("want exit 2 naming the format, got %d:\n%s", code, out)
	}
}
