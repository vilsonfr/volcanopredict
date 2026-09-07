## Purpose

Define o contrato de todo adaptador que traz dado de uma fonte externa para
dentro do sistema: as etapas obrigatórias, o que cada execução deixa
registrado, e como falha e ausência de dado são tratadas sem nunca serem
confundidas com ausência de atividade.

## ADDED Requirements

### Requirement: Ingestão atravessa etapas separadas e explícitas

Todo adaptador de fonte SHALL levar o dado por etapas distintas — busca,
parse, validação, normalização e persistência — em que nenhuma etapa assume o
trabalho de outra. Uma falha SHALL identificar em qual etapa ocorreu.

#### Scenario: Fonte responde com formato inesperado
- **WHEN** a fonte responde com um corpo que não tem a estrutura esperada
- **THEN** a execução falha na etapa de parse, nomeando-a
- **AND** nenhum registro é persistido a partir daquela resposta
- **AND** a resposta original é preservada para diagnóstico

#### Scenario: Registro individual é inválido
- **WHEN** um registro dentro de uma resposta válida tem atributo obrigatório
  ausente ou impossível
- **THEN** aquele registro é rejeitado na etapa de validação, com o motivo
- **AND** os demais registros da mesma resposta continuam sendo processados
- **AND** a execução relata a contagem de rejeitados

### Requirement: Dado cru é preservado junto do normalizado

O sistema SHALL guardar o dado original de cada registro ingerido junto do
registro normalizado, de modo que qualquer valor derivado possa ser
reconstruído a partir da origem sem consultar a fonte de novo.

#### Scenario: Reconstrução a partir da origem
- **WHEN** um registro ingerido é consultado com o dado cru pedido
- **THEN** o sistema devolve o conteúdo original tal como a fonte o entregou
- **AND** o conteúdo original permite rederivar os campos normalizados

#### Scenario: Normalização mudou de versão
- **WHEN** o parser é alterado e passa a extrair um campo de forma diferente
- **THEN** os registros já ingeridos preservam a versão do parser que os
  produziu
- **AND** é possível distinguir registro produzido antes da mudança de registro
  produzido depois

### Requirement: Toda execução de ingestão é registrada e auditável

O sistema SHALL registrar cada execução de ingestão com fonte, instante de
início, instante de fim, janela de tempo consultada, resultado, contagens de
inseridos, atualizados e rejeitados, e a mensagem de erro quando houver.

#### Scenario: Execução bem-sucedida
- **WHEN** uma ingestão termina sem erro
- **THEN** existe um registro de execução com resultado de sucesso e as
  contagens
- **AND** o registro permite saber qual janela de tempo foi coberta

#### Scenario: Execução falha no meio
- **WHEN** a fonte fica indisponível durante uma ingestão
- **THEN** existe um registro de execução com resultado de falha e a causa
- **AND** o registro NÃO é sobrescrito pela execução seguinte

#### Scenario: Consulta do estado operacional
- **WHEN** alguém pergunta o estado da ingestão de uma fonte
- **THEN** o sistema informa o resultado e o instante da última execução
- **AND** informa há quanto tempo a fonte não entrega dado com sucesso

### Requirement: Ausência de dado nunca é apresentada como ausência de atividade

Quando uma fonte não entrega dado — por falha, indisponibilidade ou lacuna na
própria fonte — o sistema SHALL representar isso como ausência de
*conhecimento*, distinta de conhecimento de que nada aconteceu. Nenhuma
resposta, agregação ou indicador SHALL apresentar período sem dado como período
sem atividade.

#### Scenario: Fonte esteve offline durante uma janela
- **WHEN** uma consulta cobre uma janela em que a ingestão da fonte falhou
- **THEN** a resposta indica que há lacuna de dado naquela janela
- **AND** a resposta NÃO apresenta a janela como período sem eventos

#### Scenario: Lacuna é identificável
- **WHEN** existe intervalo entre execuções bem-sucedidas maior que a cadência
  esperada da fonte
- **THEN** o sistema é capaz de identificar esse intervalo como lacuna
- **AND** informa seu início e seu fim

### Requirement: Falha de ingestão não degrada o serviço

Falha na ingestão de uma fonte SHALL NOT impedir a subida do sistema, derrubar
o atendimento de requisições, nem impedir a ingestão das demais fontes.

#### Scenario: Fonte indisponível na subida
- **WHEN** o sistema sobe com a fonte externa inacessível
- **THEN** o sistema passa a atender requisições normalmente
- **AND** a falha de ingestão fica registrada

#### Scenario: Erro em uma fonte com várias registradas
- **WHEN** a ingestão de uma fonte falha e outra está habilitada
- **THEN** a ingestão da outra fonte ocorre normalmente

### Requirement: Coleta automática é periódica, configurável e não se sobrepõe

O sistema SHALL coletar de cada fonte habilitada em intervalo configurável, e
SHALL garantir que uma execução para a mesma fonte não comece enquanto outra
está em andamento.

#### Scenario: Ciclo anterior ainda rodando
- **WHEN** chega o instante do próximo ciclo e o anterior não terminou
- **THEN** o novo ciclo não é iniciado para aquela fonte
- **AND** o fato fica registrado

#### Scenario: Coleta incremental
- **WHEN** uma coleta periódica ocorre depois de uma anterior bem-sucedida
- **THEN** a consulta à fonte cobre o período desde a última coleta, incluindo
  registros que a fonte revisou nesse intervalo
- **AND** registros já conhecidos e não revisados não geram versão nova

#### Scenario: Coleta manual sob demanda
- **WHEN** um operador dispara a ingestão manualmente com uma janela explícita
- **THEN** a ingestão cobre exatamente aquela janela
- **AND** produz um registro de execução como qualquer outra

### Requirement: Acesso à fonte externa é limitado e identificado

Toda requisição a uma fonte externa SHALL ter tempo limite, número máximo de
tentativas com espera crescente entre elas, e SHALL identificar este projeto no
`User-Agent`. O sistema SHALL respeitar a política de coleta declarada pela
fonte.

#### Scenario: Fonte demora a responder
- **WHEN** a fonte não responde dentro do tempo limite
- **THEN** a requisição é abortada
- **AND** a tentativa seguinte só ocorre após espera, até o limite de
  tentativas

#### Scenario: Fonte proíbe coleta automatizada
- **WHEN** uma fonte declara política que proíbe coleta automatizada, ou não
  tem licença determinada
- **THEN** o sistema não a consulta
- **AND** a fonte permanece desabilitada
