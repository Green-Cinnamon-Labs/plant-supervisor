# 04 — Reconciliacao

## O que esse operator e

Um **observador supervisorio**, nao um atuador. Ele nunca escreve na planta. Cada ciclo responde uma pergunta: a planta esta cumprindo a politica ativa? A resposta vai para o `status` do `Plant`, e quem quiser saber (kubectl, tep-ihm) le de la pela API do Kubernetes.

## Fluxo de uma avaliacao

```
Reconcile(Plant) chamado
  |
  +- 1. Busca o Plant
  |
  +- 2. Busca a OperatingPolicy de spec.policyRef
  |     -> nao existe: Pending (PolicyNotFound)
  |
  +- 3. Busca a CostFunction de policy.spec.costFunctionRef
  |     -> nao existe: Pending (CostFunctionNotFound)
  |
  +- 4. Junta os sinais necessarios (termos + metas + restricoes)
  |     e pede ao historian: POST /aggregate {keys, window_s}
  |     -> erro/timeout (5s): Pending (HistorianUnreachable)
  |     -> historian sem planta: Pending (PlantDisconnected)
  |     -> algum sinal sem amostra: Pending (MissingSignals)
  |
  +- 5. evaluate.Evaluate(costFunction, policy, medias)
  |     +- J = Σ coefficient × Π media(sinal), mais a contribuicao de cada termo
  |     +- CostWithinBudget: J <= maxCost
  |     +- TargetsMet: |media - valor| <= valor × tolerancia
  |     +- ConstraintsSatisfied: min <= media <= max
  |
  +- 6. Persistencia
  |     +- avaliacao falhou: consecutiveViolations++ ; passou: zera
  |     +- politica mudou desde a ultima avaliacao: contador recomeca
  |     +- NonCompliant se consecutiveViolations >= persistenceEvaluations
  |
  +- 7. Malhas de controle (so se a politica declara controlLoops)
  |     +- pede ao historian: POST /loop-performance {loops, window_s, sample_interval_s}
  |     |     -> erro: ControlLoopsHealthy = Unknown (HistorianUnreachable), o veredito economico fica
  |     +- evaluate.EvaluateLoops: portao σ_OP > minOutputStd, depois PI >= minPredictability
  |     +- nenhuma malha julgada: Unknown (NoLoopEvaluated), contador de malhas zera
  |     +- persistencia propria: consecutiveLoopViolations, loopPersistenceEvaluations
  |     +- ControlLoopsHealthy True/False — nunca muda a phase
  |
  +- 8. Grava status (phase, cost, terms, targets, constraints, loops, conditions)
  |
  +- 9. RequeueAfter(evaluationIntervalSeconds)
```

Os passos 5, 6 e o julgamento do passo 7 sao funcoes puras em `internal/evaluate` — o resto e encanamento. Os dois niveis de observacao (economico e qualidade das malhas) correm na mesma avaliacao, mas nao se misturam: um nao muda o veredito do outro.

## Quando o reconcile roda

- **Periodicamente**: cada avaliacao termina com `RequeueAfter(evaluationIntervalSeconds)`.
- **Quando o spec do Plant muda** (ex.: `kubectl edit plant tep` trocando `policyRef`). Mudancas so de `status` sao ignoradas (`GenerationChangedPredicate`), senao o operator se re-dispararia a cada escrita do proprio veredito.
- **Quando uma OperatingPolicy ou CostFunction muda**: todos os Plants do namespace sao reavaliados na hora, sem esperar o proximo intervalo. Simples e suficiente para o lab (poucos Plants por namespace).

## Por que o operator nao fala com a planta

O operator so conversa com o historian, nunca com a planta. Motivos:

- **Genericidade**: o operator nao precisa saber o protocolo nem a estrutura da planta. Outra planta, com outro protocolo, so precisa de um historian que responda `/aggregate`.
- **Separacao de papeis**: o historian interpreta sinais (janela, media, desvio padrao); o operator fala Kubernetes (spec, status, conditions).
- **Estatistica fora do loop do k8s**: metodos mais pesados (ex.: indices de desempenho de malha, como Harris ou o Predictability Index) cabem no historian sem mudar o operator — ele so passaria a ler um numero a mais.

## Idempotencia

Cada avaliacao recalcula tudo a partir das medias atuais; o unico estado carregado entre avaliacoes e `consecutiveViolations` (e `activePolicy`, para detectar troca de politica). Se o Pod do operator reiniciar, ele continua de onde o `status` parou.
