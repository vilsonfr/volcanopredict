# 🌋 VOLCANO PREDICTION — MASTER SPECIFICATION

**Documento:** Master Specification / Master Implementation Prompt  
**Projeto:** Volcano Prediction  
**Versão:** 0.1 — Especificação inicial consolidada  
**Status:** Planejamento / Arquitetura  
**Objetivo:** Construir uma plataforma científica de monitoramento, análise, detecção de anomalias e pesquisa de previsão de atividade vulcânica.

---

# 1. VISÃO GERAL

O **Volcano Prediction** será uma plataforma de monitoramento vulcânico multidisciplinar capaz de integrar dados provenientes de diferentes fontes, armazená-los historicamente, processá-los, detectar alterações/anomalias e apresentar os resultados em uma interface geográfica 3D.

O sistema NÃO deve começar prometendo "prever erupções".

A primeira finalidade será:

> **Construir uma representação histórica e em tempo quase real do estado dos vulcões e identificar sinais estatisticamente anormais de atividade.**

A capacidade de previsão deverá ser desenvolvida posteriormente, utilizando dados históricos, modelos estatísticos, machine learning e principalmente **back-testing temporal rigoroso**.

---

# 2. PRINCÍPIO CIENTÍFICO CENTRAL

O sistema deverá separar claramente:

1. **Observação**
2. **Medição**
3. **Processamento**
4. **Detecção de anomalias**
5. **Estimativa de atividade**
6. **Estimativa probabilística de risco**
7. **Previsão experimental**

Nunca apresentar uma previsão experimental como certeza.

O sistema deverá sempre distinguir:

- dado observado;
- dado derivado;
- anomalia;
- correlação;
- hipótese;
- previsão;
- alerta;
- confirmação científica.

---

# 3. OBJETIVO FINAL

O objetivo de longo prazo é desenvolver uma plataforma capaz de responder perguntas como:

- Quais vulcões apresentam atividade acima do comportamento normal?
- Quando essa alteração começou?
- Qual foi a magnitude da alteração?
- Quais parâmetros mudaram simultaneamente?
- Existem sinais em múltiplas fontes independentes?
- O comportamento atual é parecido com eventos históricos?
- Qual é o nível de confiança da análise?
- Qual é a incerteza?
- Quais eventos históricos apresentam padrões semelhantes?
- O modelo teria identificado o aumento de atividade antes de um evento passado?
- Quanto tempo antes?
- Com que taxa de falsos positivos?
- Como o comportamento atual se compara com o histórico daquele vulcão?

---

# 4. PRINCÍPIOS DO PROJETO

## 4.1 Dados reais primeiro

Priorizar dados reais provenientes de fontes científicas e institucionais.

Dados simulados somente poderão ser utilizados quando:

- estiverem explicitamente identificados como simulados;
- não forem misturados silenciosamente com dados reais;
- forem utilizados para testes;
- forem utilizados para desenvolvimento da interface quando necessário.

Nunca apresentar dados simulados como observações reais.

## 4.2 Histórico é fundamental

O sistema deverá armazenar não apenas o valor atual.

É necessário saber:

> "O que o sistema sabia naquele momento?"

Isso é essencial para treinamento e back-testing.

## 4.3 Não utilizar informação futura

Ao realizar análises históricas, o modelo só poderá utilizar dados que estavam disponíveis naquele instante.

Nunca permitir:

- data leakage;
- utilização acidental de eventos futuros;
- normalização baseada no futuro;
- features calculadas utilizando informações posteriores ao momento da previsão.

## 4.4 Incerteza explícita

Toda previsão ou score deverá possuir, quando aplicável:

- nível de confiança;
- intervalo;
- probabilidade;
- incerteza;
- qualidade dos dados;
- quantidade de sensores disponíveis.

## 4.5 Multidisciplinaridade

O sistema deve combinar diferentes tipos de observação:

- sismicidade;
- deformação;
- GNSS/GPS;
- InSAR;
- temperatura;
- emissões de gases;
- SO2;
- CO2;
- atividade superficial;
- imagens de satélite;
- atividade térmica;
- cinzas;
- lava;
- histórico eruptivo;
- contexto tectônico;
- meteorologia;
- outros parâmetros relevantes.

Nenhum parâmetro individual deve ser tratado automaticamente como prova de uma futura erupção.

---

# 5. ARQUITETURA GERAL

```text
                    FONTES DE DADOS
                          │
        ┌─────────────────┼──────────────────┐
        │                 │                  │
        ▼                 ▼                  ▼
     Sísmica          Satélites          GNSS/GPS
        │                 │                  │
        ├──────────────┬──┴──────────────┬───┤
        │              │                 │
        ▼              ▼                 ▼
     Gases          Térmico          Meteorologia
        │              │                 │
        └──────────────┼─────────────────┘
                       ▼
                INGESTION LAYER
                       │
                       ▼
              VALIDATION / QUALITY
                       │
                       ▼
                 NORMALIZATION
                       │
                       ▼
               TEMPORAL DATABASE
                       │
             ┌─────────┼──────────┐
             │         │          │
             ▼         ▼          ▼
         FEATURES   HISTÓRICO   RAW DATA
             │         │          │
             └─────────┼──────────┘
                       ▼
               ANALYSIS ENGINE
                       │
       ┌───────────────┼────────────────┐
       ▼               ▼                ▼
   RULE ENGINE   STATISTICAL ENGINE   ML ENGINE
       │               │                │
       └───────────────┼────────────────┘
                       ▼
               ANOMALY ENGINE
                       │
                       ▼
                 RISK ENGINE
                       │
             ┌─────────┼──────────┐
             ▼         ▼          ▼
         DASHBOARD   ALERTS    API
             │         │          │
             └─────────┼──────────┘
                       ▼
                    GLOBO 3D
```

---

# 6. COMPONENTES PRINCIPAIS

1. Data ingestion
2. Data validation
3. Data normalization
4. Data quality engine
5. Temporal database
6. Volcano database
7. Event database
8. Sensor database
9. Satellite data layer
10. Seismic data layer
11. Deformation layer
12. Gas layer
13. Thermal layer
14. Weather layer
15. Historical eruption layer
16. Feature engineering
17. Anomaly detection
18. Statistical analysis
19. Rule engine
20. Risk scoring
21. Machine learning engine
22. Back-testing engine
23. Alert engine
24. 3D globe
25. Volcano detail interface
26. Timeline
27. Historical playback
28. API
29. Authentication/authorization, caso necessário
30. Logging
31. Monitoring
32. Testing
33. Documentation
34. Model/version management

---

# 7. FASE 0 — PREPARAÇÃO DO PROJETO

Antes de implementar funcionalidades científicas, criar a estrutura base.

## 7.1 Repositório

Criar repositório Git.

Estrutura sugerida:

```text
volcano-prediction/
│
├── README.md
├── LICENSE
├── CONTRIBUTING.md
├── CHANGELOG.md
├── docker-compose.yml
├── .env.example
│
├── docs/
│   ├── architecture/
│   ├── data-sources/
│   ├── science/
│   ├── api/
│   ├── models/
│   └── backtesting/
│
├── backend/
│   ├── api/
│   ├── ingestion/
│   ├── processing/
│   ├── analysis/
│   ├── models/
│   ├── alerts/
│   └── database/
│
├── frontend/
│   ├── globe/
│   ├── dashboard/
│   ├── volcano/
│   ├── timeline/
│   ├── charts/
│   └── components/
│
├── data/
│   ├── raw/
│   ├── processed/
│   ├── fixtures/
│   └── examples/
│
├── tests/
│   ├── unit/
│   ├── integration/
│   ├── data/
│   └── backtesting/
│
└── scripts/
```

---

# 8. FASE 1 — CATÁLOGO DE VULCÕES

Criar primeiro um catálogo central de vulcões.

Cada vulcão deverá possuir:

- ID interno;
- nome;
- nomes alternativos;
- latitude;
- longitude;
- altitude;
- país;
- região;
- continente;
- tipo de vulcão;
- tipo de edifício vulcânico;
- status;
- data da última erupção conhecida;
- histórico eruptivo;
- fonte dos dados;
- observatório responsável, quando aplicável;
- links de referência;
- tectonic setting;
- placa tectônica;
- nível de atividade conhecido;
- sensores associados.

---

# 9. FASE 2 — INTEGRAÇÃO COM FONTES DE DADOS

Esta é uma das etapas mais importantes.

Antes de implementar IA, integrar fontes de dados reais.

Cada fonte deverá possuir um adaptador independente:

```text
Source Adapter
      │
      ▼
Fetcher
      │
      ▼
Parser
      │
      ▼
Validator
      │
      ▼
Normalizer
      │
      ▼
Database
```

Cada integração deverá registrar:

- origem;
- timestamp;
- timestamp de aquisição;
- timestamp do evento;
- localização;
- unidade;
- precisão;
- qualidade;
- versão do parser;
- versão da fonte;
- status;
- possíveis erros.

---

# 10. FONTES DE DADOS PRIORITÁRIAS

## 10.1 Smithsonian Global Volcanism Program

Utilizar como referência para:

- catálogo de vulcões;
- histórico eruptivo;
- eventos;
- cronologia;
- contexto histórico.

O sistema deverá armazenar a referência da fonte original.

## 10.2 Dados sísmicos

Integrar fontes sísmicas apropriadas.

Objetivos:

- terremotos próximos;
- magnitude;
- profundidade;
- localização;
- frequência;
- número de eventos;
- energia sísmica;
- padrões temporais;
- swarm detection.

Criar features como:

- número de terremotos/hora;
- número de terremotos/dia;
- média de magnitude;
- magnitude máxima;
- profundidade média;
- profundidade máxima/mínima;
- energia sísmica acumulada;
- taxa de crescimento;
- alteração do comportamento sísmico.

---

# 11. DETECÇÃO DE SWARMS

Criar mecanismo para detectar agrupamentos de terremotos.

Possíveis critérios:

- distância do vulcão;
- intervalo temporal;
- número mínimo de eventos;
- magnitude;
- profundidade;
- densidade espacial.

Os parâmetros deverão ser configuráveis.

---

# 12. DEFORMAÇÃO DO SOLO

Integrar dados disponíveis de:

- GNSS;
- GPS;
- InSAR;
- inclinômetros;
- outros sistemas de deformação.

Calcular:

- deslocamento;
- velocidade;
- aceleração;
- deformação acumulada;
- deformação vertical;
- deformação horizontal;
- mudança de tendência.

---

# 13. DADOS DE SATÉLITE

Criar camada para dados orbitais.

Possíveis informações:

- temperatura;
- anomalias térmicas;
- SO2;
- cinzas;
- plumas;
- alterações superficiais;
- deformação;
- imagens ópticas;
- radar.

O sistema deverá armazenar metadados da observação.

---

# 14. ATIVIDADE TÉRMICA

Registrar:

- temperatura observada;
- temperatura máxima;
- média;
- anomalia;
- localização;
- horário;
- sensor;
- resolução;
- cobertura de nuvens, quando aplicável.

Criar comparação com baseline histórico.

---

# 15. GASES VULCÂNICOS

Criar suporte para:

- SO2;
- CO2;
- H2S;
- outros gases disponíveis.

Armazenar:

- concentração;
- fluxo;
- unidade;
- localização;
- método de medição;
- timestamp;
- qualidade.

Calcular:

- média;
- máximo;
- tendência;
- alteração percentual;
- desvio do baseline.

---

# 16. METEOROLOGIA

Dados meteorológicos são importantes principalmente para interpretar:

- plumas;
- cinzas;
- gases;
- imagens;
- condições ambientais.

Registrar:

- vento;
- direção;
- velocidade;
- temperatura;
- pressão;
- umidade;
- precipitação.

O sistema deverá evitar interpretar automaticamente uma alteração meteorológica como atividade vulcânica.

---

# 17. TECTÔNICA

Adicionar contexto tectônico:

- placas;
- limites de placas;
- tipo de limite;
- falhas;
- contexto geológico;
- distância a estruturas relevantes;
- localização tectônica.

Esses dados devem funcionar principalmente como contexto, e não como sinal instantâneo de erupção.

---

# 18. DADOS CRUDOS

Nunca descartar os dados originais quando for possível armazená-los.

```text
RAW DATA
   │
   ├── PROCESSING
   │
   ├── NORMALIZED DATA
   │
   └── DERIVED FEATURES
```

O sistema deverá conseguir reconstruir uma feature a partir do dado original.

---

# 19. BANCO DE DADOS TEMPORAL

Para cada observação:

```text
event_time
ingestion_time
processing_time
source
sensor
value
unit
location
quality
```

É importante diferenciar:

- quando o fenômeno ocorreu;
- quando o dado foi coletado;
- quando o sistema recebeu;
- quando foi processado.

---

# 20. DATA QUALITY ENGINE

Cada observação deverá receber indicadores de qualidade.

Possíveis estados:

```text
VALID
SUSPECT
MISSING
DUPLICATE
OUTLIER
CORRUPTED
DELAYED
```

Detectar:

- valores impossíveis;
- timestamps inválidos;
- duplicatas;
- lacunas;
- mudanças abruptas causadas por sensor;
- unidades inconsistentes.

---

# 21. NORMALIZAÇÃO

Padronizar:

- unidades;
- timestamps;
- coordenadas;
- nomes;
- IDs;
- formatos.

Nunca misturar unidades sem conversão explícita.

Tudo deverá possuir unidade explícita.

---

# 22. SISTEMA DE BASELINE

Cada vulcão deverá possuir um comportamento histórico de referência.

O baseline poderá ser:

- média histórica;
- mediana;
- média móvel;
- distribuição estatística;
- sazonal;
- específica por sensor;
- específica por estação do ano;
- específica por período.

---

# 23. ANOMALIAS

Uma anomalia não significa necessariamente erupção.

Definição:

> Uma observação significativamente diferente do comportamento esperado.

Tipos:

- anomalia sísmica;
- anomalia térmica;
- anomalia de deformação;
- anomalia de gases;
- anomalia combinada.

---

# 24. DETECÇÃO DE ANOMALIAS — V1

Antes de machine learning, implementar:

- Z-score;
- média móvel;
- desvio padrão;
- percentis;
- mudança de tendência;
- EWMA;
- CUSUM;
- thresholds configuráveis.

---

# 25. ANOMALIA MULTIPARAMÉTRICA

Uma das funcionalidades centrais.

Exemplo:

```text
Sismicidade       ALTA
Deformação        ELEVADA
Temperatura       MODERADA
SO2               ALTA
Meteorologia      NORMAL
```

O sistema deverá detectar quando múltiplos indicadores apresentam alterações simultaneamente.

---

# 26. SCORE DE ATIVIDADE

Criar um score experimental de atividade.

Conceitualmente:

```text
Activity Score =
  seismic_component
+ deformation_component
+ thermal_component
+ gas_component
+ satellite_component
+ historical_context
```

Exemplo:

```text
0–20    NORMAL
20–40   ELEVATED
40–60   HIGH
60–80   VERY HIGH
80–100  EXTREME
```

Esses limites deverão ser configuráveis e não deverão ser apresentados como classificação científica oficial.

---

# 27. SCORE DE CONFIANÇA

Além do Activity Score, criar:

```text
Confidence Score
```

Considerar:

- quantidade de fontes;
- qualidade dos sensores;
- cobertura temporal;
- cobertura espacial;
- consistência entre fontes;
- disponibilidade dos dados.

Isso NÃO significa "probabilidade de erupção".

---

# 28. REGRAS CIENTÍFICAS

Criar um Rule Engine.

Exemplo:

```text
IF
  seismic_activity > threshold
AND deformation > threshold
AND gas_flux > threshold
THEN
  generate MULTI_PARAMETER_ANOMALY
```

Todas as regras deverão ser:

- versionadas;
- documentadas;
- testáveis;
- ativáveis/desativáveis;
- auditáveis.

---

# 29. GLOBO 3D

Criar uma interface global em 3D.

Permitir:

- rotação;
- zoom;
- seleção;
- pesquisa;
- filtros;
- camadas;
- timeline;
- animação;
- visualização histórica.

---

# 30. CAMADAS DO GLOBO

### Vulcões
- ativos;
- dormentes;
- históricos;
- status atual.

### Terremotos
- magnitude;
- profundidade;
- tempo;
- clusters.

### Deformação
- vetores;
- magnitude;
- tendência.

### Térmico
- hotspots;
- anomalias.

### Gases
- pontos;
- intensidade;
- tendência.

### Satélite
- imagens;
- observações.

### Tectônica
- placas;
- limites;
- falhas.

### Meteorologia
- vento;
- cobertura;
- condições.

### Cinzas
- plumas;
- direção;
- dispersão.

### Tsunami
- eventos relacionados, quando aplicável.

---

# 31. TIMELINE GLOBAL

Adicionar timeline permitindo:

- selecionar período;
- avançar;
- voltar;
- pausar;
- acelerar;
- observar eventos históricos.

---

# 32. HISTORICAL PLAYBACK

Criar reprodução histórica.

Exemplo:

> "Mostrar tudo que o sistema teria visto entre 2018 e 2019."

Isso será importante para pesquisa e back-testing.

---

# 33. PÁGINA INDIVIDUAL DO VULCÃO

Ao clicar em um vulcão, abrir dashboard específico com:

- nome;
- localização;
- altitude;
- status;
- última erupção;
- histórico;
- Activity Score;
- Confidence Score;
- sinais atuais;
- anomalias;
- terremotos;
- deformação;
- gases;
- temperatura;
- satélite;
- meteorologia;
- histórico temporal.

---

# 34. GRÁFICOS

Criar gráficos temporais:

```text
Sismicidade × tempo
Deformação × tempo
SO2 × tempo
Temperatura × tempo
Activity Score × tempo
```

Permitir sobrepor diferentes variáveis.

---

# 35. CORRELAÇÃO ENTRE VARIÁVEIS

Criar ferramentas exploratórias:

- sismicidade × deformação;
- sismicidade × gases;
- gases × temperatura;
- deformação × atividade térmica.

Correlação NÃO deve ser interpretada automaticamente como causalidade.

---

# 36. EVENTOS

Criar modelo de evento.

Tipos:

```text
EARTHQUAKE
SWARM
ERUPTION
THERMAL_ANOMALY
GAS_ANOMALY
DEFORMATION_ANOMALY
ASH
LAVA
LAHAR
TSUNAMI
OTHER
```

Cada evento deverá possuir timestamp e fonte.

---

# 37. ALERTAS

Criar sistema de alertas:

```text
INFO
WATCH
ELEVATED
HIGH
CRITICAL
```

Esses níveis serão internos/experimentais até serem validados.

Cada alerta deverá informar:

- motivo;
- fontes;
- intensidade;
- duração;
- confiança;
- timestamp;
- regra/modelo utilizado.

---

# 38. AUDITORIA DE ALERTAS

Todo alerta deverá ser armazenado.

Registrar:

- criação;
- atualização;
- encerramento;
- causa;
- modelo;
- parâmetros;
- dados utilizados.

---

# 39. MACHINE LEARNING

Machine Learning só deverá ser implementado depois de:

1. dados reais;
2. banco temporal;
3. features;
4. baseline;
5. regras;
6. anomaly detection;
7. histórico suficiente;
8. back-testing inicial.

---

# 40. FEATURES PARA ML

### Sísmicas
- eventos/hora;
- eventos/dia;
- magnitude média;
- magnitude máxima;
- energia;
- profundidade;
- swarm density;
- aceleração da atividade.

### Deformação
- deslocamento;
- velocidade;
- aceleração;
- tendência;
- deformação acumulada.

### Gases
- SO2;
- CO2;
- tendência;
- fluxo;
- alteração percentual.

### Térmico
- temperatura;
- hotspot count;
- anomalia térmica.

### Satélite
- alteração de superfície;
- presença de pluma;
- cobertura.

### Contexto
- estação;
- meteorologia;
- histórico;
- tectônica.

---

# 41. MODELOS DE ML

Testar progressivamente:

## Nível 1
Modelos estatísticos.

## Nível 2
Modelos simples:
- Logistic Regression;
- Random Forest;
- Gradient Boosting.

## Nível 3
Modelos temporais.

## Nível 4
Deep Learning, se houver dados suficientes.

Nunca utilizar um modelo complexo apenas porque é mais sofisticado.

---

# 42. TARGET DO MODELO

Definir cuidadosamente o que significa "prever".

Possíveis targets:

```text
P(activity increase in next 24h)
P(activity increase in next 7 days)
P(anomaly in next 30 days)
P(eruptive episode within X days)
```

Não começar diretamente com:

> "Vai entrar em erupção."

O target deverá ser mensurável.

---

# 43. PREVISÃO PROBABILÍSTICA

Resultado futuro poderá ser algo como:

```text
Probability of elevated activity:
Next 24h: 12%
Next 7d: 28%
Next 30d: 41%
```

Somente após validação adequada.

---

# 44. BACK-TESTING

Esta será uma das partes mais importantes do projeto.

Pergunta fundamental:

> "Se o sistema existisse no passado, ele teria percebido o aumento de atividade antes do evento?"

---

# 45. TIME-BASED BACKTEST

Nunca fazer random train/test split para séries temporais quando isso gerar vazamento temporal.

Exemplo:

```text
TRAIN
2010 ───────── 2018

VALIDATION
2019 ───── 2020

TEST
2021 ───── 2022
```

Ou utilizar walk-forward validation.

---

# 46. WALK-FORWARD TESTING

Exemplo:

```text
Train: 2010–2015
Test:  2016

Train: 2010–2016
Test:  2017

Train: 2010–2017
Test:  2018
```

Continuar progressivamente.

---

# 47. TESTE "AS-OF"

Cada previsão deverá responder:

```text
Prediction timestamp:
2020-05-10 12:00

Available data:
somente dados <= 2020-05-10 12:00
```

Nunca utilizar informações posteriores.

---

# 48. MÉTRICAS

Avaliar:

- precision;
- recall;
- F1;
- ROC-AUC;
- PR-AUC;
- false positive rate;
- false negative rate;
- detection lead time;
- calibration;
- Brier score;
- alert frequency.

---

# 49. MÉTRICA MAIS IMPORTANTE: LEAD TIME

Medir:

> Quantos dias/horas antes do evento o sistema identificou o sinal?

---

# 50. FALSOS POSITIVOS

Medir quantas vezes foram gerados alertas que não resultaram no evento esperado.

Um sistema que alerta o tempo todo não é útil.

---

# 51. FALSOS NEGATIVOS

Medir eventos que ocorreram sem alerta prévio.

---

# 52. COMPARAÇÃO COM BASELINES

Todo modelo deverá ser comparado contra modelos simples.

Exemplo:

```text
Baseline: "sempre normal"
Modelo estatístico: XX
Random Forest: XX
Modelo temporal: XX
```

Um modelo complexo precisa demonstrar melhoria real.

---

# 53. HISTÓRICO DE MODELOS

Cada modelo deverá possuir:

```text
model_id
version
training_period
features
target
hyperparameters
training_dataset
metrics
creation_date
```

---

# 54. REPRODUTIBILIDADE

Registrar:

- versão do código;
- dataset;
- parâmetros;
- features;
- random seed;
- versão do ambiente.

---

# 55. MODEL REGISTRY

Manter:

```text
Model v0.1
Model v0.2
Model v0.3
...
```

Nunca substituir silenciosamente um modelo antigo.

---

# 56. EXPLICAÇÃO DO MODELO

Quando possível, mostrar quais fatores contribuíram para a previsão.

Exemplo:

```text
Main contributors:

Sismicity       +31%
Deformation     +24%
SO2             +19%
Thermal         +11%
Historical      +7%
```

Tratar isso como explicabilidade do modelo, não como causalidade física.

---

# 57. DETECÇÃO DE MUDANÇA DE REGIME

Futuramente identificar:

> "O comportamento do vulcão mudou de regime?"

Exemplo:

```text
NORMAL
   ↓
ELEVATED
   ↓
ACCELERATING
   ↓
HIGH ACTIVITY
```

---

# 58. COMPARAÇÃO COM EVENTOS HISTÓRICOS

Criar mecanismo para encontrar períodos historicamente semelhantes.

A similaridade deverá ser quantitativa e documentada.

Nunca dizer que similaridade significa que o evento ocorrerá novamente.

---

# 59. DETECÇÃO DE ANOMALIAS SEM SUPERVISÃO

Futuramente testar:

- Isolation Forest;
- clustering;
- autoencoders;
- métodos de mudança de distribuição.

Somente depois de possuir baseline sólido.

---

# 60. SISTEMA DE EXPERIMENTOS

Cada experimento deverá registrar:

```text
experiment_id
dataset
features
model
parameters
results
date
author
notes
```

---

# 61. DATA VERSIONING

Datasets utilizados em modelos devem ser versionados.

Exemplo:

```text
dataset-2026-01
dataset-2026-02
dataset-2026-03
```

---

# 62. API

Criar API para:

- listar vulcões;
- consultar vulcão;
- consultar eventos;
- consultar terremotos;
- consultar anomalias;
- consultar scores;
- consultar histórico;
- consultar alertas;
- consultar modelos;
- executar análises;
- consultar back-tests.

Endpoints conceituais:

```text
GET /volcanoes
GET /volcanoes/{id}
GET /volcanoes/{id}/events
GET /volcanoes/{id}/seismic
GET /volcanoes/{id}/deformation
GET /volcanoes/{id}/gas
GET /volcanoes/{id}/thermal
GET /volcanoes/{id}/anomalies
GET /volcanoes/{id}/score
GET /volcanoes/{id}/history

GET /alerts
GET /models
GET /experiments
GET /backtests
```

---

# 63. FRONTEND

Priorizar:

- clareza;
- velocidade;
- informação científica;
- visualização temporal;
- exploração.

Evitar transformar o sistema em uma interface excessivamente "gamer".

---

# 64. DASHBOARD PRINCIPAL

Mostrar:

```text
GLOBAL STATUS

Active volcanoes
Recent earthquakes
Current anomalies
High activity volcanoes
Recent eruptions
Satellite observations
Alerts
```

---

# 65. PESQUISA

Permitir pesquisar:

- vulcão;
- país;
- região;
- evento;
- coordenadas;
- período.

---

# 66. FILTROS

Filtros por:

- atividade;
- continente;
- país;
- magnitude;
- tipo de evento;
- período;
- score;
- nível de confiança;
- fonte.

---

# 67. COMPARAÇÃO DE VULCÕES

Permitir comparar dois ou mais vulcões.

Comparar:

- atividade;
- sismicidade;
- deformação;
- gases;
- temperatura;
- histórico;
- scores.

---

# 68. COMPARAÇÃO TEMPORAL

Permitir:

```text
Atual
vs
30 dias atrás
vs
1 ano atrás
vs
episódio histórico
```

---

# 69. MODO CIENTÍFICO

Criar interface avançada para pesquisadores mostrando:

- dados brutos;
- dados normalizados;
- features;
- qualidade;
- timestamps;
- fontes;
- modelos;
- resultados estatísticos.

---

# 70. MODO SIMPLIFICADO

Criar interface para usuários não especialistas mostrando:

- status;
- tendência;
- principais sinais;
- explicação simples;
- fontes.

---

# 71. TRANSPARÊNCIA

Cada informação importante deverá indicar:

```text
Source
Timestamp
Last update
Data quality
Method
```

---

# 72. SISTEMA DE FONTES

Manter catálogo:

```text
source_id
name
organization
url
type
update_frequency
license
coverage
reliability_notes
```

---

# 73. MONITORAMENTO DA INGESTÃO

Dashboard interno para saber se as fontes estão funcionando.

Exemplo:

```text
USGS             ONLINE
Satellite API    ONLINE
GNSS             DELAYED
Gas data         OFFLINE
Weather          ONLINE
```

---

# 74. DETECÇÃO DE FALHAS

Se uma fonte ficar offline:

- registrar;
- alertar administrador;
- não interpretar ausência de dados como ausência de atividade.

---

# 75. DATA GAPS

Identificar lacunas.

Exemplo:

```text
GNSS data gap:
2026-04-01 → 2026-04-03
```

---

# 76. SEGURANÇA

Implementar:

- secrets via environment variables;
- autenticação para áreas administrativas;
- autorização;
- rate limiting;
- validação de entrada;
- logs;
- proteção de APIs.

Nunca colocar credenciais no código.

---

# 77. LOGGING

Registrar:

- ingestion;
- processamento;
- erros;
- análises;
- alertas;
- execução de modelos;
- backtests.

---

# 78. OBSERVABILIDADE

Monitorar:

- CPU;
- memória;
- latência;
- APIs;
- filas;
- banco;
- ingestão;
- erros.

---

# 79. TESTES

Cada componente deverá possuir testes:

## Unitários
Funções individualmente.

## Integração
Pipeline completo.

## Dados
Qualidade e schemas.

## API
Endpoints.

## Frontend
Componentes críticos.

## Científicos
Cálculos e métricas.

---

# 80. TESTES CIENTÍFICOS

Criar datasets conhecidos para verificar:

- Z-score;
- médias;
- tendências;
- energia sísmica;
- deformação;
- scores;
- probabilidades;
- métricas de back-test.

---

# 81. TESTES DE DATA LEAKAGE

Criar testes específicos para garantir que:

```text
feature(t)
```

não utiliza:

```text
data(t + 1)
```

ou qualquer dado futuro.

---

# 82. TESTES DE REPRODUTIBILIDADE

Executar o mesmo experimento duas vezes.

Resultado deverá ser igual ou explicavelmente diferente devido a aleatoriedade controlada.

---

# 83. DOCUMENTAÇÃO

Documentar:

- arquitetura;
- APIs;
- banco;
- fontes;
- pipelines;
- modelos;
- métricas;
- decisões científicas;
- limitações.

---

# 84. LIMITAÇÕES CIENTÍFICAS

A documentação deverá declarar explicitamente:

> **Não existe garantia de previsão de erupções vulcânicas.**

O sistema é uma plataforma experimental de monitoramento e análise.

---

# 85. IMPORTANTE SOBRE ALERTAS

O Volcano Prediction não deve substituir:

- observatórios vulcanológicos;
- autoridades;
- sistemas oficiais de emergência;
- comunicados científicos.

Alertas experimentais deverão ser claramente identificados.

---

# 86. FASEAMENTO DO DESENVOLVIMENTO

## V0.1 — Fundação

Implementar:

- repositório;
- arquitetura;
- banco;
- catálogo de vulcões;
- API básica;
- frontend básico;
- logging;
- testes.

## V0.2 — Dados Reais

Implementar:

- primeiras APIs/fontes;
- ingestão;
- normalização;
- armazenamento temporal;
- qualidade dos dados;
- atualização automática.

## V0.3 — Globo 3D

Implementar:

- globo;
- vulcões;
- terremotos;
- filtros;
- seleção;
- timeline;
- histórico.

## V0.4 — Dashboard Científico

Implementar:

- página individual;
- gráficos;
- histórico;
- comparação;
- múltiplas camadas.

## V0.5 — Anomaly Engine

Implementar:

- baseline;
- Z-score;
- médias móveis;
- thresholds;
- detecção multiparamétrica;
- Activity Score;
- Confidence Score.

## V0.6 — Back-testing

Implementar:

- datasets históricos;
- as-of analysis;
- walk-forward;
- métricas;
- lead time;
- falsos positivos;
- falsos negativos.

## V0.7 — Statistical Engine

Implementar:

- modelos probabilísticos;
- mudança de regime;
- correlações;
- análise temporal;
- comparação histórica.

## V0.8 — Machine Learning

Implementar:

- feature store;
- treinamento;
- validação;
- modelos;
- model registry;
- explicabilidade.

## V0.9 — Prediction Engine

Implementar previsões probabilísticas experimentais.

## V1.0 — Plataforma Integrada

Integrar:

```text
DATA
 ↓
PROCESSING
 ↓
ANOMALY
 ↓
STATISTICS
 ↓
ML
 ↓
BACKTEST
 ↓
PREDICTION
 ↓
VISUALIZATION
 ↓
ALERTS
```

---

# 87. ORDEM DE IMPLEMENTAÇÃO OBRIGATÓRIA

Não começar pela IA.

Seguir esta ordem:

```text
1. Repository
2. Architecture
3. Database
4. Volcano catalog
5. Data source registry
6. First real data integration
7. Data ingestion
8. Validation
9. Normalization
10. Temporal storage
11. Data quality
12. API
13. Frontend foundation
14. 3D globe
15. Volcano dashboard
16. Timeline
17. Historical playback
18. Baselines
19. Basic anomaly detection
20. Multi-parameter analysis
21. Activity Score
22. Confidence Score
23. Alerts
24. Historical datasets
25. Back-testing
26. Statistical models
27. Feature engineering
28. Machine learning
29. Prediction experiments
30. Production hardening
```

---

# 88. CRITÉRIO DE CONCLUSÃO DE CADA FASE

Uma fase NÃO deve ser considerada concluída apenas porque "o código funciona".

Cada fase deverá possuir:

- implementação;
- testes;
- tratamento de erros;
- documentação;
- dados de exemplo;
- validação;
- logs;
- README atualizado.

---

# 89. REGRA PARA O AGENTE DE IMPLEMENTAÇÃO

Ao implementar este projeto:

1. Não pular etapas.
2. Não implementar funcionalidades futuras prematuramente.
3. Não inventar dados.
4. Não inventar APIs.
5. Não assumir que uma fonte possui determinado dado sem verificar.
6. Não esconder erros de ingestão.
7. Não misturar dados reais e simulados.
8. Não usar dados futuros em análises históricas.
9. Não apresentar correlação como causalidade.
10. Não apresentar score experimental como previsão oficial.
11. Não criar uma "IA de previsão" antes de possuir dataset adequado.
12. Não apagar dados históricos.
13. Versionar modelos.
14. Versionar datasets.
15. Registrar todas as fontes.
16. Documentar decisões científicas.
17. Criar testes para cada componente crítico.

---

# 90. PRINCÍPIO DE DESENVOLVIMENTO INCREMENTAL

Cada funcionalidade deverá ser construída em pequenos incrementos.

Exemplo:

```text
Fonte sísmica
    ↓
Fetcher
    ↓
Parser
    ↓
Validator
    ↓
Database
    ↓
API
    ↓
Chart
    ↓
Globe
```

Só depois integrar essa fonte ao mecanismo de análise.

---

# 91. ARQUITETURA MODULAR

Cada fonte deve ser substituível.

Exemplo:

```text
interfaces/
    seismic_provider
    satellite_provider
    gas_provider
    deformation_provider
```

Assim, uma fonte poderá ser substituída sem reescrever o sistema inteiro.

---

# 92. EVENT SOURCING / AUDIT TRAIL

Sempre que possível, preservar o histórico das mudanças.

Exemplo:

```text
Observation received
      ↓
Observation corrected
      ↓
Observation reprocessed
      ↓
Feature recalculated
      ↓
Model rerun
```

Manter rastreabilidade.

---

# 93. CACHE

Utilizar cache quando apropriado para:

- dados que mudam pouco;
- catálogo de vulcões;
- mapas;
- tiles;
- resultados de consultas frequentes.

Não utilizar cache de forma que dados em tempo real fiquem excessivamente atrasados.

---

# 94. PROCESSAMENTO ASSÍNCRONO

Para tarefas pesadas, considerar filas:

```text
Ingestion Queue
Processing Queue
Analysis Queue
ML Queue
Alert Queue
```

---

# 95. ESCALABILIDADE

A arquitetura deverá permitir adicionar:

```text
10 vulcões
100 vulcões
1.000 vulcões
10.000+ vulcões
```

sem precisar reconstruir o sistema inteiro.

---

# 96. MULTI-RESOLUTION

Dados deverão poder ser consultados em diferentes resoluções:

```text
raw
minute
hour
day
week
month
```

Sem destruir o dado original.

---

# 97. AGREGAÇÕES TEMPORAIS

Calcular:

- média;
- mediana;
- mínimo;
- máximo;
- desvio;
- percentis;
- tendência;
- contagem;
- energia acumulada.

---

# 98. DETECÇÃO DE OUTLIERS

Separar:

```text
Physical anomaly
Sensor anomaly
Data error
```

Um sensor defeituoso não deve gerar automaticamente um alerta vulcânico.

---

# 99. SENSOR FUSION

Futuramente combinar múltiplos sensores.

Exemplo:

```text
Sensor A → anormal
Sensor B → anormal
Sensor C → normal
Sensor D → anormal
```

O sistema deverá avaliar consistência.

---

# 100. CONFIABILIDADE DA FONTE

Cada fonte poderá possuir metadata de:

```text
coverage
latency
quality
availability
resolution
```

Esses fatores podem entrar no Confidence Score.

---

# 101. GEOESPACIAL

Todos os dados geográficos deverão possuir:

- latitude;
- longitude;
- sistema de coordenadas;
- timestamp.

Quando necessário:

- altitude;
- profundidade;
- geometria;
- bounding box.

---

# 102. MAPA DE INCERTEZA

Futuramente criar visualização espacial de incerteza.

---

# 103. MODEL CARD

Para cada modelo de ML criar documentação contendo:

- objetivo;
- dataset;
- período;
- features;
- target;
- limitações;
- métricas;
- falsos positivos;
- falsos negativos;
- casos de falha.

---

# 104. DATA CARD

Para cada dataset importante documentar:

- origem;
- período;
- cobertura;
- unidades;
- limitações;
- missing data;
- licença;
- processamento aplicado.

---

# 105. EXPERIMENTAL LAB

Criar uma área para pesquisadores testarem:

```text
Dataset
+
Features
+
Algorithm
+
Parameters
=
Experiment
```

Gerar relatório automaticamente.

---

# 106. RELATÓRIO DE BACK-TEST

Gerar relatório contendo:

```text
Model
Dataset
Period
Events
Predictions
True positives
False positives
False negatives
Precision
Recall
F1
Lead time
Calibration
```

---

# 107. VISUALIZAÇÃO DO BACK-TEST

Mostrar timeline:

```text
atividade
│
│        anomaly
│       ▲
│      ▲
│     ▲
│                 ERUPTION
│                    ▲
└────────────────────────────── time
```

Isso permitirá visualizar se o sinal apareceu antes do evento.

---

# 108. COMPARAÇÃO DE MODELOS

Permitir comparar:

```text
Rule Engine
vs
Statistical Model
vs
Random Forest
vs
Gradient Boosting
vs
Temporal Model
```

---

# 109. DETECÇÃO DE DRIFT

Monitorar se o comportamento dos dados muda.

```text
Training distribution
        ↓
Current distribution
        ↓
SIGNIFICANT DRIFT
```

Se houver drift, o modelo poderá precisar ser reavaliado.

---

# 110. RETRAINING

Não treinar novamente automaticamente sem controle.

Criar processo:

```text
New data
 ↓
Quality check
 ↓
Drift check
 ↓
Candidate model
 ↓
Back-test
 ↓
Validation
 ↓
Approval
 ↓
Deployment
```

---

# 111. DEPLOYMENT DE MODELOS

Manter:

```text
Production Model
Candidate Model
Archived Models
```

Nunca substituir o modelo de produção sem validação.

---

# 112. ALERTAS DE MODELO

Criar alertas internos para:

- degradação;
- drift;
- aumento de falsos positivos;
- indisponibilidade;
- erro de inferência.

---

# 113. EXPLICAÇÃO AO USUÁRIO

Nunca mostrar apenas:

```text
RISK = 83
```

Mostrar:

```text
Activity level: HIGH

Main signals:
• seismic activity increased
• deformation trend increased
• thermal anomaly detected

Confidence: medium/high

This is an experimental analytical signal and is not an official eruption forecast.
```

---

# 114. FONTES OFICIAIS

Quando existir informação oficial relevante, priorizar fontes institucionais.

O sistema deverá manter links e referências das fontes.

---

# 115. LICENÇAS

Antes de integrar qualquer dataset:

- verificar licença;
- verificar termos de uso;
- verificar atribuição;
- verificar limites de API;
- verificar redistribuição.

Documentar tudo.

---

# 116. RATE LIMITING DE FONTES

Respeitar limites das APIs.

Implementar:

- retry;
- exponential backoff;
- caching;
- request throttling.

---

# 117. RESILIÊNCIA

Uma fonte offline não deve derrubar a plataforma inteira.

```text
Source A ── OK
Source B ── OK
Source C ── OFFLINE
Source D ── OK

Sistema continua funcionando.
```

---

# 118. STATUS DO SISTEMA

Dashboard administrativo:

```text
Data ingestion
Database
API
Analysis Engine
Alert Engine
ML Engine
Globe
```

Cada componente:

```text
ONLINE
DEGRADED
OFFLINE
```

---

# 119. ROADMAP FUTURO

Após V1.0, considerar:

- previsão probabilística avançada;
- modelos multimodais;
- análise de imagens;
- computer vision;
- detecção automática de plumas;
- análise de séries sísmicas;
- modelos espaciais;
- grafos de sensores;
- modelos de propagação;
- previsão de cinzas;
- integração com riscos de aviação;
- integração com tsunami;
- análise de lahars;
- simulações físicas;
- digital twin de vulcões.

Essas funcionalidades não fazem parte obrigatoriamente da primeira versão.

---

# 120. POSSÍVEL DIGITAL TWIN

Futuramente cada vulcão poderá possuir uma representação digital:

```text
VOLCANO DIGITAL TWIN

Geology
+
History
+
Sensors
+
Satellite
+
Seismicity
+
Deformation
+
Gas
+
Thermal
+
Models
```

---

# 121. PRINCIPAL DIFERENCIAL DO PROJETO

O diferencial não deve ser simplesmente:

> "Um mapa com vulcões."

O objetivo é:

> **Criar uma plataforma temporal e multidisciplinar que permita observar, comparar, detectar anomalias, testar hipóteses e avaliar retrospectivamente sinais de atividade vulcânica.**

O foco será:

```text
DATA
 ↓
CONTEXT
 ↓
ANOMALY
 ↓
ANALYSIS
 ↓
UNCERTAINTY
 ↓
BACKTEST
 ↓
PREDICTION
```

---

# 122. FILOSOFIA CIENTÍFICA

O projeto deverá seguir o princípio:

> **Primeiro medir. Depois entender. Depois testar. Só então prever.**

Não começar pela resposta.

Começar pelos dados.

---

# 123. CHECKLIST FINAL DE IMPLEMENTAÇÃO

## Fundação

- [ ] Git
- [ ] estrutura do projeto
- [ ] documentação
- [ ] ambiente
- [ ] configuração
- [ ] Docker, se aplicável

## Dados

- [ ] catálogo de vulcões
- [ ] registry de fontes
- [ ] integração sísmica
- [ ] integração GNSS
- [ ] integração InSAR
- [ ] integração satélite
- [ ] integração térmica
- [ ] integração gases
- [ ] integração meteorológica
- [ ] dados tectônicos
- [ ] histórico eruptivo

## Pipeline

- [ ] ingestion
- [ ] validation
- [ ] normalization
- [ ] quality
- [ ] temporal storage
- [ ] raw data
- [ ] processed data
- [ ] feature store

## Visualização

- [ ] globo 3D
- [ ] vulcões
- [ ] terremotos
- [ ] satélite
- [ ] térmico
- [ ] gases
- [ ] deformação
- [ ] tectônica
- [ ] timeline
- [ ] playback
- [ ] filtros
- [ ] pesquisa

## Análise

- [ ] baseline
- [ ] Z-score
- [ ] moving average
- [ ] EWMA
- [ ] CUSUM
- [ ] trend detection
- [ ] swarm detection
- [ ] anomaly engine
- [ ] multi-parameter analysis
- [ ] Activity Score
- [ ] Confidence Score

## Histórico

- [ ] eventos
- [ ] histórico eruptivo
- [ ] comparação temporal
- [ ] eventos semelhantes
- [ ] playback

## Back-testing

- [ ] temporal split
- [ ] walk-forward
- [ ] as-of data
- [ ] leakage tests
- [ ] precision
- [ ] recall
- [ ] F1
- [ ] ROC-AUC
- [ ] PR-AUC
- [ ] calibration
- [ ] false positives
- [ ] false negatives
- [ ] lead time

## Machine Learning

- [ ] feature engineering
- [ ] datasets versionados
- [ ] modelos baseline
- [ ] Random Forest
- [ ] Gradient Boosting
- [ ] modelos temporais
- [ ] model registry
- [ ] model cards
- [ ] explicabilidade
- [ ] drift detection
- [ ] retraining controlado

## Produção

- [ ] logging
- [ ] monitoring
- [ ] security
- [ ] API
- [ ] rate limiting
- [ ] error handling
- [ ] backup
- [ ] observabilidade
- [ ] documentação

---

# 124. REGRA FINAL PARA IMPLEMENTAÇÃO

O agente responsável pela implementação deverá seguir este documento como **Master Specification**.

Antes de implementar uma funcionalidade:

1. verificar em qual fase ela pertence;
2. verificar dependências;
3. implementar a menor versão funcional;
4. criar testes;
5. validar dados;
6. documentar;
7. somente então avançar.

Se uma funcionalidade futura depender de dados ainda não disponíveis, NÃO criar uma falsa implementação.

Criar:

```text
interface
+
mock
+
test
+
TODO
```

quando apropriado.

---

# 125. DEFINIÇÃO DE SUCESSO

A primeira grande vitória do projeto não será prever uma erupção.

Será conseguir abrir o sistema e responder:

> "O que está acontecendo com os vulcões do planeta agora, quais sinais mudaram, quando mudaram, quais dados sustentam essa conclusão e quão confiável é essa análise?"

Depois disso:

> "O que teria acontecido se estivéssemos olhando para os mesmos dados cinco anos atrás?"

E somente depois:

> "Podemos construir um modelo probabilístico que demonstre desempenho melhor que os baselines?"

---

# 126. META FINAL

Construir uma plataforma científica de exploração vulcânica que una:

🌋 Vulcões  
🌎 Globo 3D  
🌐 Dados globais  
📡 Sensores  
🛰️ Satélites  
🌡️ Temperatura  
💨 Gases  
🌐 GNSS/InSAR  
🌎 Tectônica  
🌦️ Meteorologia  
📈 Séries temporais  
🧠 Estatística  
🤖 Machine Learning  
🔬 Detecção de anomalias  
⏪ Histórico  
🧪 Back-testing  
📊 Probabilidade  
⚠️ Alertas experimentais  
🔎 Explicabilidade  
📚 Reprodutibilidade  

em uma única plataforma.

---

# 127. INSTRUÇÃO PARA O PRIMEIRO AGENTE DE IMPLEMENTAÇÃO

Comece SOMENTE pela V0.1.

Não implementar Machine Learning ainda.

Primeiro:

```text
1. Criar estrutura do projeto
2. Configurar ambiente
3. Criar banco temporal
4. Criar modelo de Vulcano
5. Criar modelo de Event
6. Criar modelo de Observation
7. Criar Source Registry
8. Criar API básica
9. Criar testes
10. Criar documentação
```

Depois disso, parar e validar a arquitetura.

Em seguida começar a primeira integração real de dados.

A implementação deverá ser incremental, testável, observável e cientificamente rastreável.

---

# FIM DA MASTER SPECIFICATION

**Projeto:** Volcano Prediction  
**Documento:** VOLCANO_PREDICTION_MASTER_SPEC.md  
**Versão:** 0.1

> "Não construir primeiro uma máquina que tenta adivinhar o futuro.  
> Construir primeiro uma máquina que consiga reconstruir o passado, observar o presente e medir sua própria incerteza."
