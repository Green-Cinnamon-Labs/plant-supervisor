# 03 — As CRDs

Tres tipos no grupo `supervision.greenlabs.io/v1alpha1`, todos namespaced. Eles se referenciam por nome dentro do mesmo namespace:

```
Plant ──policyRef──▶ OperatingPolicy ──costFunctionRef──▶ CostFunction
```

| Kind              | Responde a pergunta                        | Quem escreve |
|-------------------|--------------------------------------------|--------------|
| `CostFunction`    | Como se calcula o custo J desta planta?    | Usuario (spec) |
| `OperatingPolicy` | O que conta como "operar bem" agora?       | Usuario (spec) |
| `Plant`           | Qual politica vale, e ela esta sendo cumprida? | Usuario (spec) e operator (status) |

## CostFunction

A funcao objetivo, declarada como soma de termos. Cada termo e `coefficient × Π media(sinal)`:

```yaml
spec:
  unit: "$/h"
  reference: 170.6            # J em operacao nominal (opcional, so para comparacao)
  source: "Downs & Vogel (1993), Table 9"
  terms:
    - name: purge.a
      coefficient: 0.9880674  # 2.206 $/kgmol × 44.79 kgmol/h por kscmh / 100 (mol% → fracao)
      signals: [xmeas.stream9.flow_rate, xmeas.stream9.component.a]
    - name: compressor
      coefficient: 0.0536     # $/kWh
      signals: [xmeas.compressor.work]
```

- Um sinal: termo linear (preco × potencia). Dois sinais: produto (preco × vazao × composicao).
- Conversao de unidade fica **dentro do coeficiente** — o operator so multiplica e soma.
- Os nomes de sinal sao as chaves que o historian conhece (browse names OPC-UA da planta).
- **Aproximacao conhecida**: um termo produto usa `media(a) × media(b)`, nao `media(a × b)`. Os dois coincidem quando os sinais estao estaveis na janela, que e o regime que um custo economico descreve.

Exemplo completo (os 12 termos do TEP): `tep-supervisor/local/k8s/tep/cost-function-downs-vogel.yaml`.

## OperatingPolicy

Uma maneira de operar a planta. Equivale a um "modo" de Downs & Vogel somado a um orcamento de custo:

```yaml
spec:
  costFunctionRef: tep-downs-vogel
  windowSeconds: 60            # janela das medias pedidas ao historian
  maxCost: 179.0               # orcamento de J (opcional: sem ele, J e so observado)
  persistenceEvaluations: 3    # quantas avaliacoes ruins seguidas ate declarar NonCompliant
  targets:                     # media do sinal dentro de ±tolerancePercent do valor
    - signal: xmeas.stream11.flow_rate
      value: 22.949
      tolerancePercent: 5
  constraints:                 # media do sinal dentro de [min, max]; qualquer limite pode faltar
    - signal: xmeas.reactor.pressure
      max: 2895
```

`persistenceEvaluations` vem do CLPM (Bradu et al. 2018): um transiente curto nao deve virar veredito. As avaliacoes sao janelas deslizantes (uma a cada `evaluationIntervalSeconds` do Plant, cada uma olhando os ultimos `windowSeconds`), por isso o nome conta avaliacoes e nao janelas.

## Plant

O que o usuario declara e minimo:

```yaml
spec:
  historianURL: "http://host.docker.internal:8090"
  policyRef: tep-mode1          # trocar de politica = trocar esse campo
  evaluationIntervalSeconds: 30
```

O `status` e o veredito, escrito pelo operator:

```yaml
status:
  phase: Compliant              # Pending | Compliant | NonCompliant
  activePolicy: tep-mode1
  cost: {value: 166.39, unit: "$/h", reference: 170.6, maxCost: 179}
  terms: [{name: purge.a, value: 10.7}, ...]          # contribuicao de cada termo para J
  targets: [{signal: ..., target: 22.949, observed: 22.64, met: true}, ...]
  constraints: [{signal: ..., observed: 2695.1, satisfied: true}, ...]
  consecutiveViolations: 0
  lastEvaluationTime: "..."
  conditions:
    - type: DataAvailable          # historian alcancavel, planta conectada, todos os sinais com amostra
    - type: CostWithinBudget       # J <= maxCost
    - type: TargetsMet
    - type: ConstraintsSatisfied
    - type: PolicyCompliant        # o veredito, ja filtrado pela persistencia
```

```
$ kubectl get plants
NAME   POLICY      COST      UNIT   PHASE       AGE
tep    tep-mode1   166.39    $/h    Compliant   5m
```

### Fases

| Phase          | Quando |
|----------------|--------|
| `Pending`      | Sem veredito possivel: politica/funcao de custo inexistente, historian fora do ar, planta desconectada ou sinal sem amostra. As conditions de veredito ficam `Unknown` (nunca um True/False velho). O motivo esta em `DataAvailable.reason`. |
| `Compliant`    | Todas as checagens passam, ou falham ha menos de `persistenceEvaluations` avaliacoes seguidas. |
| `NonCompliant` | Falharam `persistenceEvaluations` avaliacoes seguidas. |

Trocar a politica ativa zera o contador de violacoes: violacoes da politica anterior nao dizem nada sobre a nova.
