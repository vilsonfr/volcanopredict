## Purpose

Define o modelo de dados bitemporal do VolcanoPredict: como cada observação registra separadamente quando o fenômeno ocorreu e quando o sistema tomou conhecimento dele, como se reconstrói o estado do conhecimento em qualquer instante passado, e como a proveniência real versus sintética é marcada de forma que nunca se perca.

Esta capacidade é a pré-condição para todo back-testing honesto: sem ela, não é possível responder "o que o sistema sabia naquele momento?" e qualquer avaliação de modelo estará contaminada por informação futura.

## ADDED Requirements

### Requirement: Todo registro temporal é bitemporal

Toda observação e todo evento armazenado SHALL registrar dois instantes distintos e independentes:

- **tempo do fenômeno** — quando o fato ocorreu no mundo, conforme a fonte;
- **tempo de conhecimento** — quando o registro passou a estar disponível no sistema.

O tempo de conhecimento SHALL ser atribuído pelo sistema no momento da persistência e SHALL ser imutável depois disso. Nenhum registro SHALL ser aceito sem ambos os instantes.

#### Scenario: Ingestão de dado atrasado

- **WHEN** uma observação referente a um fenômeno de três dias atrás é persistida hoje
- **THEN** seu tempo de fenômeno é a data de três dias atrás e seu tempo de conhecimento é o instante atual, e os dois valores diferem

#### Scenario: Tempo de conhecimento não é fornecido pelo cliente

- **WHEN** uma requisição de escrita tenta especificar explicitamente o tempo de conhecimento de um registro
- **THEN** o valor fornecido é ignorado e o sistema atribui o instante da persistência

#### Scenario: Tempo de fenômeno ausente

- **WHEN** uma tentativa de persistir observação sem tempo de fenômeno ocorre
- **THEN** a escrita é rejeitada com erro, e nenhum registro parcial é gravado

#### Scenario: Todos os instantes em UTC

- **WHEN** um instante é persistido ou lido, qualquer que seja o fuso da fonte original
- **THEN** ele é armazenado e retornado em UTC com fuso explícito, nunca como horário local ambíguo

### Requirement: Consulta as-of reconstrói o conhecimento de um instante passado

O sistema SHALL permitir consultar observações restringindo-as ao que era conhecido em um instante passado informado. Uma consulta as-of SHALL retornar exclusivamente registros cujo tempo de conhecimento seja anterior ou igual ao instante consultado, independentemente de seu tempo de fenômeno.

#### Scenario: Registro ingerido depois do instante consultado é invisível

- **WHEN** uma consulta as-of para o instante T é executada, e existe uma observação com tempo de fenômeno anterior a T mas tempo de conhecimento posterior a T
- **THEN** essa observação não aparece no resultado

#### Scenario: Consulta as-of é determinística

- **WHEN** a mesma consulta as-of para o instante T é executada duas vezes, em momentos diferentes, sem que nenhum registro anterior a T tenha sido alterado
- **THEN** ambas as execuções retornam exatamente o mesmo conjunto de registros

#### Scenario: Ausência de instante retorna o estado atual

- **WHEN** uma consulta é executada sem informar instante as-of
- **THEN** o resultado reflete todo o conhecimento presente, equivalente a uma consulta as-of no instante atual

#### Scenario: Consulta as-of no futuro é rejeitada

- **WHEN** uma consulta as-of informa um instante posterior ao instante atual
- **THEN** a consulta é rejeitada com erro, em vez de retornar silenciosamente o estado atual

### Requirement: Histórico é preservado por correção, nunca por sobrescrita

Quando uma fonte revisa um dado já ingerido, o sistema SHALL persistir a versão revisada como registro novo, preservando a versão anterior com seu tempo de conhecimento original. Registros históricos SHALL NOT ser atualizados nem apagados.

#### Scenario: Fonte revisa a magnitude de um sismo

- **WHEN** uma fonte publica magnitude revisada para um sismo já ingerido, e a revisão é persistida
- **THEN** ambas as versões existem no banco, uma consulta as-of anterior à revisão retorna a magnitude original, e uma consulta as-of posterior retorna a revisada

#### Scenario: Leitura atual escolhe a versão mais recente

- **WHEN** um registro possui múltiplas versões e é lido sem instante as-of
- **THEN** apenas a versão de maior tempo de conhecimento é retornada, e não uma duplicata por versão

### Requirement: Proveniência real ou sintética é obrigatória e nunca se perde

Todo registro de observação e de evento SHALL carregar uma marcação obrigatória indicando se o dado é uma observação real de uma fonte científica ou um dado sintético. A marcação SHALL ser propagada íntegra para toda leitura, incluindo respostas de API e exportações. Não SHALL existir valor padrão implícito nem estado nulo para essa marcação.

#### Scenario: Escrita sem marcação de proveniência

- **WHEN** uma tentativa de persistir observação sem a marcação de proveniência ocorre
- **THEN** a escrita é rejeitada, e nenhum registro é gravado

#### Scenario: Dado sintético sempre identificável na leitura

- **WHEN** um registro sintético é retornado em qualquer leitura do sistema
- **THEN** ele vem acompanhado da marcação que o identifica como sintético

#### Scenario: Filtro por proveniência

- **WHEN** uma consulta pede exclusivamente dados reais
- **THEN** nenhum registro sintético aparece no resultado, mesmo que satisfaça todos os demais critérios

#### Scenario: Agregação não mistura proveniências silenciosamente

- **WHEN** uma consulta agregada abrange registros reais e sintéticos
- **THEN** ou o resultado é segmentado por proveniência, ou a resposta declara explicitamente que houve mistura — nunca um número único sem qualificação

### Requirement: Toda observação é rastreável até sua fonte

Todo registro de observação e de evento SHALL referenciar a fonte de dados de onde veio, e essa referência SHALL apontar para uma fonte registrada no catálogo de fontes.

#### Scenario: Referência a fonte inexistente

- **WHEN** uma tentativa de persistir observação referenciando uma fonte que não existe no catálogo ocorre
- **THEN** a escrita é rejeitada

#### Scenario: Rastreabilidade na leitura

- **WHEN** uma observação é lida
- **THEN** é possível identificar a partir dela qual fonte a originou

### Requirement: Consultas temporais têm desempenho garantido por índice

Consultas que filtram por intervalo de tempo de fenômeno, por tempo de conhecimento, ou por ambos, SHALL ser servidas por índices, sem varredura completa de tabela.

#### Scenario: Consulta as-of por janela

- **WHEN** o plano de execução de uma consulta as-of que filtra por janela de tempo de fenômeno é inspecionado
- **THEN** ele utiliza índice, e não varredura sequencial da tabela
