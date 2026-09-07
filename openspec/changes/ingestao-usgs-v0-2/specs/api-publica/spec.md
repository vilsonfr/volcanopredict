## ADDED Requirements

### Requirement: Coleção de sismos servida sob o contrato existente

A API SHALL expor os sismos ingeridos como coleção versionada, sob o mesmo envelope, a mesma paginação determinística e o mesmo formato de erro das demais coleções. A coleção SHALL aceitar filtro por janela de ocorrência, instante as-of, proximidade geográfica, magnitude mínima, proveniência e estado de qualidade.

#### Scenario: Listagem paginada de sismos

- **WHEN** um consumidor percorre a coleção de sismos por inteiro usando o cursor
- **THEN** nenhum sismo é repetido nem omitido

#### Scenario: Consulta as-of sobre sismos

- **WHEN** uma requisição informa um instante as-of
- **THEN** nenhum sismo com instante de conhecimento posterior ao pedido é retornado

#### Scenario: Sismos próximos a um ponto

- **WHEN** a requisição informa latitude, longitude e raio
- **THEN** apenas sismos dentro do raio são retornados, com a distância em cada item

#### Scenario: Parâmetro geoespacial incompleto

- **WHEN** a requisição informa parte dos parâmetros de proximidade e omite os demais
- **THEN** a resposta é um erro que nomeia o parâmetro faltante

### Requirement: Estado de qualidade acompanha o dado servido

Todo registro ingerido retornado pela API SHALL trazer seu estado de qualidade. A API SHALL aceitar filtro por estado de qualidade, e SHALL NOT devolver conjunto de qualidade mista sem que a composição seja observável por quem lê.

#### Scenario: Registro suspeito é servido marcado

- **WHEN** um registro com estado diferente de `valid` é retornado
- **THEN** a resposta traz o estado e o motivo daquele registro

#### Scenario: Consumidor pede apenas registros válidos

- **WHEN** uma requisição filtra por qualidade `valid`
- **THEN** nenhum registro de outro estado é retornado

#### Scenario: Composição de qualidade do conjunto

- **WHEN** uma resposta contém registros de estados de qualidade diferentes
- **THEN** a resposta permite saber quantos registros de cada estado a compõem

### Requirement: Cobertura de ingestão acompanha resposta sobre janela de tempo

Toda resposta que cubra uma janela de tempo sobre dado ingerido SHALL declarar a cobertura de ingestão daquela janela. Coleção vazia SHALL ser distinguível entre "não houve evento" e "não houve coleta".

#### Scenario: Janela sem coleta bem-sucedida

- **WHEN** uma consulta cobre janela em que a ingestão da fonte falhou
- **THEN** a resposta declara cobertura parcial e identifica o intervalo não coberto
- **AND** a resposta NÃO apresenta a janela como período sem eventos

#### Scenario: Coleção vazia com cobertura completa

- **WHEN** uma consulta cobre janela integralmente coletada e nenhum evento a satisfaz
- **THEN** a resposta é uma coleção vazia declarando cobertura completa

### Requirement: Estado da ingestão é consultável

A API SHALL expor, para cada fonte habilitada, o resultado e o instante da última execução de ingestão e há quanto tempo a fonte não entrega dado com sucesso.

#### Scenario: Fonte com ingestão falhando

- **WHEN** a última execução de uma fonte falhou
- **THEN** a resposta informa a falha, seu instante e a última coleta bem-sucedida

#### Scenario: Nenhuma informação interna vaza

- **WHEN** a causa da falha de ingestão é consultada
- **THEN** a resposta descreve a falha sem expor SQL, caminho de arquivo, credencial nem rastro de pilha

## MODIFIED Requirements

### Requirement: Toda resposta expõe proveniência do dado

Todo registro de observação ou evento retornado pela API SHALL indicar se é dado real ou sintético, e SHALL permitir identificar a fonte que o originou. A API SHALL aceitar filtro por proveniência.

Todo registro originado de ingestão externa SHALL permitir chegar também ao dado cru tal como a fonte o entregou, e à versão do parser que o produziu, de modo que qualquer valor servido possa ser reconciliado com sua origem.

#### Scenario: Resposta contendo dado sintético

- **WHEN** um endpoint retorna registros dos quais ao menos um é sintético
- **THEN** cada registro sintético vem marcado como tal na resposta

#### Scenario: Consumidor pede apenas dado real

- **WHEN** uma requisição filtra por dados reais
- **THEN** nenhum registro sintético é retornado

#### Scenario: Ausência de dado real não é preenchida com sintético

- **WHEN** não há dado real satisfazendo uma consulta
- **THEN** a resposta é uma coleção vazia, e não um conjunto de dados sintéticos de preenchimento

#### Scenario: Reconciliação com a origem

- **WHEN** um consumidor pede o dado cru de um registro ingerido
- **THEN** a resposta traz o conteúdo original entregue pela fonte
- **AND** traz a versão do parser que produziu os campos normalizados
