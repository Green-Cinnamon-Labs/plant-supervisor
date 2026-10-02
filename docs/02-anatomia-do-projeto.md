# 02 — Anatomia do projeto

O Kubebuilder gera um monte de arquivo quando voce roda o scaffold. A maioria e boilerplate que voce nunca mexe. Esse doc separa o que importa do que e ruido.

## Mapa de arquivos

### Arquivos que voce EDITA

```
api/v1alpha1/
  costfunction_types.go       ← CONTRATO: a funcao de custo declarada (lista de termos).
  operatingpolicy_types.go    ← CONTRATO: metas, restricoes, orcamento, persistencia.
  plant_types.go              ← CONTRATO: qual politica esta ativa + o veredito (status).
  groupversion_info.go        ← Grupo supervision.greenlabs.io/v1alpha1.
internal/
  evaluate/                   ← A CONTA. Funcoes puras: J, metas, restricoes, persistencia.
  historian/                  ← Cliente HTTP do tep-historian (POST /aggregate).
  controller/
    plant_controller.go       ← O CORACAO. Busca os objetos, chama historian e evaluate, grava status.
config/samples/               ← Exemplos genericos (nao-TEP) das 3 CRDs.
cmd/main.go                   ← Entry point do manager. Registra o PlantReconciler.
```

A separacao `evaluate` x `controller` e de proposito: toda a aritmetica da tese (J, veredito) esta em `internal/evaluate`, sem Kubernetes nem HTTP, e e la que os testes se concentram. O controller so faz encanamento.

### Arquivos AUTO-GERADOS (nao edite)

Regenerados toda vez que voce roda `make generate manifests`:

```
api/v1alpha1/zz_generated.deepcopy.go    ← DeepCopy. Gerado a partir dos types.
config/crd/bases/supervision.greenlabs.io_*.yaml  ← YAML das 3 CRDs. Gerado dos markers +kubebuilder.
config/rbac/role.yaml                    ← Permissoes do controller. Gerado dos markers +kubebuilder:rbac.
```

**Regra de ouro**: se o arquivo tem `DO NOT EDIT` no topo ou comeca com `zz_`, nao mexe nele. Edita o source (types.go ou controller.go) e roda `make generate manifests`.

### Outros arquivos

```
hack/boilerplate.go.txt     ← Header de licenca pros arquivos gerados. controller-gen precisa dele.
PROJECT                     ← Metadados do Kubebuilder.
go.mod / go.sum             ← Dependencias Go.
```

## Os markers `+kubebuilder:`

Esses comentarios magicos nos arquivos Go nao sao decoracao — o `controller-gen` le eles e gera o CRD YAML, RBAC, validacoes, etc. Por isso eles tem que continuar sendo comentarios `//`.

Exemplos nos types:

```go
// +kubebuilder:validation:MinItems=1           → lista nao pode ser vazia
// +kubebuilder:validation:Minimum=1            → min no schema
// +kubebuilder:default=60                      → valor default no schema
// +kubebuilder:subresource:status              → habilita /status subresource (so no Plant)
// +kubebuilder:printcolumn:name="Cost",...     → colunas no kubectl get
// +listType=map / +listMapKey=name             → itens de lista identificados por chave
```

E no `plant_controller.go`:

```go
// +kubebuilder:rbac:groups=supervision.greenlabs.io,resources=plants,...
```

**Ciclo de vida**: edita os markers → roda `make generate manifests` → YAML atualizado.

## Fluxo de build

```
make generate     → gera zz_generated.deepcopy.go
make manifests    → gera CRD YAML + RBAC YAML a partir dos markers
make test         → testes unitarios + envtest (baixa etcd/kube-apiserver em bin/)
make build        → compila o binario do manager
docker build -t tep-operator:latest .
```

**Windows**: `make generate manifests` funciona, mas o Makefile passa os pacotes explicitamente (`paths="./api/v1alpha1" paths="./internal/controller"`) porque `paths="./..."` falha no Windows ("no Go files"). Se criar um pacote novo com markers, adicione o path no Makefile. O envtest tambem roda no Windows; o unico porem e que ele nao consegue encerrar etcd/kube-apiserver no fim (o `AfterSuite` ignora esse erro so no Windows).

## O que foi removido do scaffold original

O Kubebuilder gera um projeto pronto pra producao com CI, monitoring, linting pesado, etc.
Pra um lab, isso e peso morto. Removemos em marco/2026:

| Removido                 | O que era                                      | Porque saiu                                                     |
| ------------------------ | ---------------------------------------------- | --------------------------------------------------------------- |
| `.github/workflows/`     | CI (lint, test, e2e)                           | Nao esta configurado pra esse repo. Recria quando tiver CI real |
| `.devcontainer/`         | Config de Codespace/devcontainer               | Nao usamos Codespaces                                           |
| `config/prometheus/`     | ServiceMonitor pro Prometheus                  | Monitoring vem depois, se precisar                              |
| `config/network-policy/` | NetworkPolicy K8s                              | Desnecessario num Kind local                                    |
| `.golangci.yml`          | Config do golangci-lint (24 linters)           | Overkill pro lab. `go vet` resolve                              |
| `.custom-gcl.yml`        | Plugin custom do linter (logcheck)             | Overkill                                                        |
| `AGENTS.md`              | Guia generico do Kubebuilder pra agentes de IA | Substituido pelos nossos `docs/`                                |

Tambem simplificamos `config/default/kustomization.yaml` — era 235 linhas de boilerplate
comentado (webhooks, cert-manager, etc). Ficou com ~15 linhas, so o que esta ativo.

**Se precisar de volta**: tudo que foi removido e padrao do Kubebuilder. Basta rodar
`kubebuilder init` num repo limpo pra ver os templates, ou consultar o
[Kubebuilder Book](https://book.kubebuilder.io/).
