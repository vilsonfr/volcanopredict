## Purpose

Define o contrato HTTP público do VolcanoPredict na versão 1 — formato de resposta, tratamento de erro, paginação, filtro geoespacial e temporal — e a obrigação, válida para todo endpoint, de expor proveniência do dado e nunca apresentar resultado experimental como alerta oficial.

## Requirements

### Requirement: Contrato versionado e estável

Todo endpoint público SHALL residir sob um prefixo de versão. Alterações incompatíveis ao formato de resposta de uma versão publicada SHALL exigir uma versão nova, e não modificação da existente.

#### Scenario: Endpoint fora da versão

- **WHEN** uma requisição atinge um caminho da API sem prefixo de versão
- **THEN** ela recebe `404`, e não uma resposta de uma versão implícita

### Requirement: Endpoint de saúde reflete dependências

O sistema SHALL expor um endpoint de saúde que reporte `200` somente quando o backend estiver apto a servir requisições, incluindo conectividade com o banco de dados e schema na versão esperada.

#### Scenario: Banco indisponível

- **WHEN** o banco de dados está inalcançável e o endpoint de saúde é consultado
- **THEN** ele responde com status de indisponibilidade, e não `200`

#### Scenario: Schema desatualizado

- **WHEN** o backend está conectado a um banco cuja versão de schema é anterior à esperada
- **THEN** o endpoint de saúde não reporta `200`

### Requirement: Erros são estruturados e não vazam interno

Toda resposta de erro SHALL usar um formato único, contendo um código de erro estável legível por máquina e uma mensagem legível por humano. Respostas de erro SHALL NOT expor detalhes internos como consultas SQL, caminhos de arquivo ou rastros de pilha.

#### Scenario: Parâmetro inválido

- **WHEN** uma requisição envia um parâmetro de consulta com valor inválido
- **THEN** a resposta é `400` no formato de erro padrão, nomeando o parâmetro rejeitado

#### Scenario: Falha interna

- **WHEN** uma falha inesperada ocorre no processamento
- **THEN** a resposta é `500` no formato padrão com mensagem genérica, e o detalhe técnico vai apenas para o log do servidor, correlacionável por identificador presente na resposta

### Requirement: Listagens são paginadas e determinísticas

Todo endpoint que retorne coleção SHALL ser paginado, SHALL declarar um limite máximo de itens por página, e SHALL ter ordenação total definida, de modo que a paginação não repita nem omita itens.

#### Scenario: Limite acima do máximo

- **WHEN** uma requisição pede mais itens por página do que o máximo permitido
- **THEN** a requisição é rejeitada com `400`, em vez de silenciosamente reduzida

#### Scenario: Paginação não repete nem omite

- **WHEN** uma coleção é percorrida página a página até o fim, sem escrita concorrente
- **THEN** cada item aparece exatamente uma vez no conjunto das páginas

#### Scenario: Coleção vazia

- **WHEN** nenhum registro satisfaz os filtros de uma listagem
- **THEN** a resposta é `200` com coleção vazia, e não `404`

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

### Requirement: Consulta geoespacial e temporal sobre os dados

A API SHALL permitir filtrar vulcões por proximidade a um ponto geográfico e filtrar observações e eventos por janela de tempo de fenômeno, bem como reconstruir o conhecimento disponível em um instante passado.

#### Scenario: Busca por raio

- **WHEN** uma requisição pede vulcões dentro de um raio a partir de uma coordenada
- **THEN** o resultado contém apenas vulcões dentro do raio, ordenados por distância crescente, com a distância presente em cada item

#### Scenario: Coordenada fora de faixa

- **WHEN** uma requisição informa latitude ou longitude fora das faixas válidas
- **THEN** a resposta é `400` no formato de erro padrão

#### Scenario: Janela temporal invertida

- **WHEN** uma requisição informa início de janela posterior ao fim
- **THEN** a resposta é `400`, e não uma coleção vazia

#### Scenario: Reconstrução de conhecimento passado

- **WHEN** uma requisição informa um instante as-of
- **THEN** o resultado contém apenas registros que já eram conhecidos pelo sistema naquele instante

### Requirement: Nenhum resultado é apresentado como alerta oficial

Toda resposta que contenha score, estimativa de risco ou resultado analítico derivado SHALL vir acompanhada de declaração de que se trata de resultado experimental de pesquisa e não constitui alerta oficial de emergência.

#### Scenario: Resposta com resultado derivado

- **WHEN** um endpoint retorna qualquer valor derivado de análise, e não observação direta
- **THEN** a resposta carrega a ressalva de que o valor é experimental e não substitui autoridade oficial

#### Scenario: Ressalva não é removível por parâmetro

- **WHEN** uma requisição tenta suprimir a ressalva por parâmetro de consulta ou cabeçalho
- **THEN** a ressalva permanece na resposta

### Requirement: Toda requisição é observável

Toda requisição SHALL ser registrada em log estruturado com método, caminho, status, duração e um identificador de correlação. Logs SHALL NOT conter segredos nem chaves de API.

#### Scenario: Correlação entre erro e log

- **WHEN** uma requisição resulta em `500` e retorna um identificador de correlação
- **THEN** o log do servidor contém uma entrada com esse mesmo identificador e o detalhe técnico da falha

#### Scenario: Segredo em parâmetro

- **WHEN** uma requisição carrega um valor sensível em parâmetro de consulta
- **THEN** o valor aparece redigido no log, e não em texto claro

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
