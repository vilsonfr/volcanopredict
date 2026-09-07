## Purpose

Define o catálogo de fontes externas de dados do VolcanoPredict — que fontes existem, sob qual licença seu dado pode ser usado, que atribuição é devida, e com que cadência são atualizadas.

É pré-condição para qualquer conector de ingestão: nenhum dado externo entra no sistema sem que sua origem esteja registrada e suas condições de uso sejam conhecidas.

## ADDED Requirements

### Requirement: Toda fonte externa é registrada antes de ser usada

Nenhum dado externo SHALL ser ingerido a partir de uma fonte que não esteja registrada no catálogo de fontes. O registro SHALL preceder a ingestão.

#### Scenario: Ingestão de fonte não registrada

- **WHEN** um processo de ingestão tenta persistir dado atribuído a uma fonte ausente do catálogo
- **THEN** a operação é rejeitada com erro que nomeia a fonte desconhecida

### Requirement: Registro de fonte carrega condições de uso

Cada fonte registrada SHALL declarar, no mínimo: nome, URL base, categoria de dado, identificador de licença, texto de atribuição exigido pela fonte, e se está habilitada para ingestão. Uma fonte SHALL NOT ser marcada como habilitada enquanto sua licença e atribuição não estiverem preenchidas.

#### Scenario: Habilitar fonte sem licença

- **WHEN** uma tentativa de marcar como habilitada uma fonte cujo campo de licença está vazio ocorre
- **THEN** a operação é rejeitada

#### Scenario: Fonte registrada mas não habilitada

- **WHEN** uma fonte existe no catálogo com licença ainda não determinada e permanece desabilitada
- **THEN** ela é visível no catálogo para fins de documentação, mas nenhuma ingestão a partir dela é permitida

### Requirement: Atribuição é exposta junto do dado

Toda leitura que sirva dado derivado de uma fonte externa SHALL permitir identificar a atribuição devida a essa fonte.

#### Scenario: Consumidor da API descobre a atribuição

- **WHEN** um consumidor recebe registros originados de uma fonte externa
- **THEN** ele consegue, a partir da resposta, chegar ao texto de atribuição exigido por aquela fonte

### Requirement: Catálogo de fontes é auditável

O catálogo SHALL registrar quando cada fonte foi adicionada e quando seu registro foi alterado pela última vez. Fontes SHALL NOT ser removidas do catálogo enquanto existirem dados atribuídos a elas.

#### Scenario: Tentativa de remover fonte em uso

- **WHEN** uma tentativa de remover uma fonte que possui observações associadas ocorre
- **THEN** a remoção é rejeitada, e a fonte pode no máximo ser desabilitada

#### Scenario: Alteração de registro é datada

- **WHEN** o registro de uma fonte é alterado
- **THEN** o instante da alteração é atualizado no catálogo
