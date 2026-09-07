## Purpose

Define os indicadores de qualidade que todo registro ingerido carrega, o que
cada estado significa, e a regra que os torna úteis: registro duvidoso é
marcado e preservado, nunca descartado em silêncio nem promovido a bom.

## Requirements

### Requirement: Todo registro ingerido carrega um estado de qualidade

Todo registro persistido pela ingestão SHALL carregar um estado de qualidade —
`valid`, `suspect`, `duplicate`, `outlier`, `corrupted` ou `delayed` — e, quando
o estado não for `valid`, o motivo que o produziu. O estado SHALL NOT ser
opcional nem ter valor padrão implícito na escrita.

#### Scenario: Persistência sem estado de qualidade
- **WHEN** uma escrita tenta persistir registro sem estado de qualidade
- **THEN** a escrita é rejeitada

#### Scenario: Estado não-válido carrega motivo
- **WHEN** um registro recebe estado diferente de `valid`
- **THEN** o motivo fica gravado junto do registro
- **AND** o motivo identifica qual verificação produziu aquele estado

### Requirement: Registro duvidoso é preservado, nunca descartado

O sistema SHALL persistir registro que falhou verificação de qualidade,
marcando-o com o estado correspondente, em vez de descartá-lo. Apenas registro
que não pôde sequer ser interpretado SHALL ser recusado, e ainda assim SHALL
ficar registrado como rejeição contabilizada.

#### Scenario: Valor implausível
- **WHEN** um registro chega com valor fora da faixa fisicamente possível para
  a grandeza
- **THEN** o registro é persistido com estado `suspect` ou `outlier` e o motivo
- **AND** NÃO é descartado

#### Scenario: Registro ilegível
- **WHEN** um registro não pode ser interpretado
- **THEN** é recusado com estado `corrupted` registrado na execução
- **AND** a contagem de rejeitados da execução o inclui

### Requirement: Consumidor nunca lê registro duvidoso como bom

Toda leitura que sirva registro ingerido SHALL expor o estado de qualidade
junto do dado. Nenhuma resposta, agregação ou valor derivado SHALL misturar
registro `valid` com registro de outro estado sem que a distinção seja
observável por quem lê.

#### Scenario: Leitura devolve o estado
- **WHEN** um registro é lido pela API
- **THEN** a resposta traz o estado de qualidade daquele registro

#### Scenario: Filtro por qualidade
- **WHEN** um consumidor pede apenas registros de qualidade `valid`
- **THEN** nenhum registro de outro estado é devolvido

#### Scenario: Agregação sobre qualidade mista
- **WHEN** um valor é derivado de um conjunto que contém registro não-`valid`
- **THEN** a resposta declara que o conjunto tem qualidade mista
- **AND** permite saber quantos registros de cada estado o compõem

### Requirement: Verificações de qualidade são explícitas e nomeadas

O sistema SHALL aplicar, a cada registro ingerido, verificações de valor
impossível, timestamp inválido, duplicata e chegada tardia. Cada verificação
SHALL ter nome estável que apareça no motivo gravado, de modo que a razão de um
estado seja rastreável até a regra que o produziu.

#### Scenario: Timestamp no futuro
- **WHEN** um registro chega com instante de ocorrência posterior ao instante
  da coleta
- **THEN** recebe estado `suspect` com motivo que nomeia a verificação de
  timestamp

#### Scenario: Mesmo registro repetido na mesma resposta
- **WHEN** a fonte entrega duas vezes o mesmo registro na mesma resposta
- **THEN** a segunda ocorrência recebe estado `duplicate`
- **AND** não cria versão nova do registro já persistido

#### Scenario: Chegada muito posterior à ocorrência
- **WHEN** um registro chega com atraso maior que o esperado para a cadência da
  fonte
- **THEN** recebe estado `delayed`
- **AND** permanece visível na consulta as-of a partir do instante em que
  chegou, nunca antes

### Requirement: Qualidade é atributo da versão, não do registro eterno

Quando a fonte revisa um registro, a nova versão SHALL receber sua própria
avaliação de qualidade. Reavaliar não SHALL alterar o estado gravado em versão
anterior.

#### Scenario: Revisão corrige valor implausível
- **WHEN** a fonte revisa um registro que estava `suspect` e o novo valor é
  plausível
- **THEN** a versão nova é persistida com estado `valid`
- **AND** a versão anterior continua com estado `suspect`
- **AND** uma consulta as-of anterior à revisão continua devolvendo `suspect`
