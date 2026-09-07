## Purpose

Define o que o sistema guarda sobre um sismo vindo de fonte externa, quais
atributos são obrigatórios, como a revisão de um evento pela fonte é
representada sem apagar o que se sabia antes, e como esse dado é consultável.

## ADDED Requirements

### Requirement: Sismo tem identidade estável dada pela fonte

Cada sismo SHALL ser identificado pelo identificador da própria fonte,
escopado pela fonte que o forneceu. O sistema SHALL NOT reescrever, renumerar
nem inferir identificador próprio que substitua o da origem.

#### Scenario: Mesmo evento reingerido
- **WHEN** a mesma execução ou uma execução posterior traz um evento já
  conhecido, sem alteração
- **THEN** nenhuma versão nova é criada
- **AND** o identificador do evento permanece o mesmo

#### Scenario: Fontes diferentes com identificadores iguais
- **WHEN** duas fontes usam o mesmo identificador para eventos distintos
- **THEN** os dois eventos coexistem sem colidir

### Requirement: Atributos obrigatórios e unidades explícitas

Todo sismo persistido SHALL ter instante de ocorrência, localização
geoespacial e proveniência real ou sintética. Magnitude, profundidade e tipo de
magnitude SHALL ser guardados quando a fonte os fornecer, e SHALL ser
distinguíveis de valor ausente. Toda grandeza SHALL ter unidade explícita e
SHALL NOT ser convertida implicitamente.

#### Scenario: Evento sem magnitude
- **WHEN** a fonte entrega um evento sem magnitude
- **THEN** o evento é persistido com magnitude ausente
- **AND** a ausência é distinguível de magnitude zero

#### Scenario: Escrita sem proveniência
- **WHEN** uma escrita omite se o sismo é real ou sintético
- **THEN** a escrita é rejeitada

#### Scenario: Profundidade em unidade conhecida
- **WHEN** um sismo é lido
- **THEN** a profundidade vem acompanhada de sua unidade
- **AND** a unidade é a mesma para todo sismo, independente da fonte

### Requirement: Revisão pela fonte cria versão, nunca sobrescreve

Quando a fonte revisa um evento — corrigindo magnitude, profundidade,
localização ou promovendo-o de solução automática para revisada — o sistema
SHALL persistir uma versão nova e SHALL preservar a anterior. A leitura
corrente SHALL devolver a versão mais recente, e a leitura as-of SHALL devolver
a versão vigente no instante consultado.

#### Scenario: Magnitude corrigida pela fonte
- **WHEN** a fonte publica magnitude diferente para um evento já ingerido
- **THEN** existe uma versão nova com a magnitude nova
- **AND** a versão anterior continua acessível
- **AND** a leitura corrente devolve apenas a versão nova

#### Scenario: Consulta as-of anterior à revisão
- **WHEN** uma consulta as-of pede o estado num instante anterior à revisão
- **THEN** devolve a magnitude que era conhecida naquele instante
- **AND** NÃO devolve a magnitude revisada

#### Scenario: Status da solução muda
- **WHEN** um evento é promovido de solução automática para revisada
- **THEN** o status da fonte fica gravado em cada versão
- **AND** é possível distinguir versão automática de versão revisada

### Requirement: Sismos são consultáveis por tempo, espaço e qualidade

O sistema SHALL permitir consultar sismos por janela de tempo de ocorrência,
por instante as-of, por proximidade geográfica a um ponto, por magnitude
mínima, por proveniência e por estado de qualidade.

#### Scenario: Sismos próximos a um ponto
- **WHEN** a consulta informa um ponto e um raio
- **THEN** devolve apenas sismos dentro do raio
- **AND** cada item traz a distância até o ponto

#### Scenario: Janela de tempo invertida
- **WHEN** o início da janela é posterior ao fim
- **THEN** a consulta é rejeitada com erro que nomeia o parâmetro

#### Scenario: Proveniência não é misturada em silêncio
- **WHEN** uma consulta não indica proveniência
- **THEN** devolve apenas sismos reais

### Requirement: Contagem de sismos nunca afirma o que não foi observado

Qualquer contagem, taxa ou agregação sobre sismos numa janela SHALL declarar a
cobertura de ingestão daquela janela. Janela com lacuna de ingestão SHALL NOT
produzir contagem apresentada como completa.

#### Scenario: Janela com lacuna
- **WHEN** uma contagem cobre janela em que a ingestão da fonte falhou
- **THEN** o resultado declara que a cobertura é parcial
- **AND** identifica o intervalo não coberto

#### Scenario: Janela integralmente coberta
- **WHEN** uma contagem cobre janela com ingestão bem-sucedida do começo ao fim
- **THEN** o resultado declara cobertura completa
