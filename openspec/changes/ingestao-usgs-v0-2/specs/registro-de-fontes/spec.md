## MODIFIED Requirements

### Requirement: Registro de fonte carrega condições de uso

Cada fonte registrada SHALL declarar, no mínimo: nome, URL base, categoria de dado, identificador de licença, texto de atribuição exigido pela fonte, e se está habilitada para ingestão. Uma fonte SHALL NOT ser marcada como habilitada enquanto sua licença e atribuição não estiverem preenchidas.

O registro SHALL declarar também a cadência de coleta com que o sistema consulta a fonte, distinta da cadência com que a fonte publica, e SHALL registrar a política de coleta declarada pela fonte quando ela existir. Fonte cuja política proíba coleta automatizada SHALL NOT ser habilitada, ainda que sua licença seja permissiva.

#### Scenario: Habilitar fonte sem licença

- **WHEN** uma tentativa de marcar como habilitada uma fonte cujo campo de licença está vazio ocorre
- **THEN** a operação é rejeitada

#### Scenario: Fonte registrada mas não habilitada

- **WHEN** uma fonte existe no catálogo com licença ainda não determinada e permanece desabilitada
- **THEN** ela é visível no catálogo para fins de documentação, mas nenhuma ingestão a partir dela é permitida

#### Scenario: Fonte proíbe coleta automatizada

- **WHEN** uma fonte declara política que proíbe coleta automatizada de seu conteúdo
- **THEN** ela não pode ser habilitada
- **AND** o motivo do bloqueio fica registrado junto da fonte

#### Scenario: Cadência de coleta é declarada

- **WHEN** uma fonte é habilitada para ingestão
- **THEN** seu registro declara com que frequência o sistema a consulta
- **AND** essa frequência é distinguível da cadência com que a fonte publica

### Requirement: Catálogo de fontes é auditável

O catálogo SHALL registrar quando cada fonte foi adicionada e quando seu registro foi alterado pela última vez. Fontes SHALL NOT ser removidas do catálogo enquanto existirem dados atribuídos a elas.

O catálogo SHALL expor também o estado operacional de cada fonte habilitada: o resultado e o instante da última execução de ingestão, e há quanto tempo a fonte não entrega dado com sucesso. Fonte habilitada que nunca foi coletada SHALL ser distinguível de fonte cuja última coleta falhou.

#### Scenario: Tentativa de remover fonte em uso

- **WHEN** uma tentativa de remover uma fonte que possui observações associadas ocorre
- **THEN** a remoção é rejeitada, e a fonte pode no máximo ser desabilitada

#### Scenario: Alteração de registro é datada

- **WHEN** o registro de uma fonte é alterado
- **THEN** o instante da alteração é atualizado no catálogo

#### Scenario: Estado operacional após falha

- **WHEN** a última ingestão de uma fonte habilitada falhou
- **THEN** o catálogo informa a falha e o instante em que ocorreu
- **AND** informa qual foi a última coleta bem-sucedida, se houve

#### Scenario: Fonte habilitada e nunca coletada

- **WHEN** uma fonte foi habilitada e nenhuma ingestão foi executada ainda
- **THEN** o catálogo a distingue de uma fonte cuja coleta falhou
