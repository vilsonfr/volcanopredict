## Purpose

Define o cadastro de vulcões do VolcanoPredict — sua origem no Smithsonian Global Volcanism Program, a identidade estável de cada vulcão ao longo de reimportações, e a rastreabilidade de cada campo até a fonte que o forneceu.

Este é o primeiro dado real do projeto e a espinha à qual toda observação futura será ancorada.

## ADDED Requirements

### Requirement: Catálogo importado de fonte científica reconhecida

O catálogo de vulcões SHALL ser populado a partir do Smithsonian Global Volcanism Program. Nenhum vulcão SHALL ser inserido a partir de conhecimento não atribuído ou de dado inventado.

#### Scenario: Importação inicial

- **WHEN** a importação do catálogo é executada sobre um banco sem vulcões
- **THEN** os vulcões do catálogo da fonte são persistidos, e a quantidade importada é registrada em log

#### Scenario: Registro de vulcão exige atribuição

- **WHEN** uma tentativa de inserir vulcão sem referência à fonte de origem ocorre
- **THEN** a inserção é rejeitada

### Requirement: Identidade estável entre reimportações

Cada vulcão SHALL ter um identificador estável derivado do identificador atribuído pela fonte de origem. Esse identificador SHALL permanecer o mesmo entre execuções sucessivas da importação.

#### Scenario: Reimportação preserva identidade

- **WHEN** a importação é executada uma segunda vez sobre um catálogo já populado
- **THEN** cada vulcão mantém o mesmo identificador da primeira execução, e nenhuma duplicata é criada

#### Scenario: Referências permanecem válidas

- **WHEN** observações foram associadas a vulcões e a importação é reexecutada
- **THEN** todas as associações continuam apontando para os mesmos vulcões

### Requirement: Importação é idempotente e atualiza sem destruir

Executar a importação repetidamente SHALL convergir para o mesmo estado. Vulcões novos na fonte SHALL ser inseridos; vulcões cujos atributos mudaram na fonte SHALL ser atualizados; vulcões que desapareceram da fonte SHALL NOT ser apagados, mas marcados como ausentes na origem.

#### Scenario: Importação sem mudanças na fonte

- **WHEN** a importação roda duas vezes seguidas sem que a fonte tenha mudado
- **THEN** a segunda execução não altera nenhum registro e relata zero inserções e zero atualizações

#### Scenario: Vulcão sai do catálogo da fonte

- **WHEN** uma importação ocorre e um vulcão anteriormente presente não aparece mais na fonte
- **THEN** o registro permanece no banco, marcado como ausente na origem, e observações associadas a ele continuam íntegras

#### Scenario: Atributo mudou na fonte

- **WHEN** a fonte passa a reportar elevação diferente para um vulcão já cadastrado
- **THEN** o registro é atualizado com o novo valor

### Requirement: Atributos geoespaciais válidos e consultáveis

Todo vulcão SHALL ter uma posição geográfica válida em coordenadas WGS84. Coordenadas fora de faixa ou ausentes SHALL fazer o registro ser rejeitado, e não silenciosamente corrigido.

#### Scenario: Coordenada inválida na fonte

- **WHEN** a fonte fornece um vulcão com latitude fora da faixa de -90 a 90
- **THEN** esse registro é rejeitado, o erro é registrado em log identificando o vulcão, e a importação prossegue com os demais registros

#### Scenario: Consulta por proximidade

- **WHEN** uma consulta pede os vulcões dentro de um raio a partir de um ponto geográfico
- **THEN** o resultado contém exatamente os vulcões cuja posição está dentro do raio, ordenados por distância crescente

### Requirement: Falha parcial de importação é visível e não corrompe o catálogo

Uma importação que encontra registros inválidos SHALL processar os registros válidos, relatar contagem de sucessos e falhas ao final, e SHALL NOT deixar o catálogo em estado parcialmente atualizado sem que isso seja reportado.

#### Scenario: Fonte indisponível

- **WHEN** a fonte não pode ser alcançada durante a importação
- **THEN** a importação falha com erro explícito, o catálogo existente permanece inalterado, e o erro não é silenciado

#### Scenario: Relatório ao final

- **WHEN** uma importação conclui
- **THEN** ela relata quantos registros foram inseridos, atualizados, inalterados e rejeitados
