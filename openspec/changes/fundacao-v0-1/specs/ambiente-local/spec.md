## Purpose

Define como o VolcanoPredict sobe, se configura e migra seu banco numa máquina de desenvolvimento, de modo que qualquer pessoa consiga um ambiente idêntico e funcional a partir de um clone limpo do repositório.

## ADDED Requirements

### Requirement: Subida completa a partir de clone limpo

O sistema SHALL subir por completo — banco, backend e frontend — a partir de um clone limpo do repositório, com um único comando documentado no README, sem passos manuais adicionais além de copiar o arquivo de exemplo de configuração.

#### Scenario: Clone limpo sobe sem intervenção

- **WHEN** uma pessoa clona o repositório num diretório vazio, copia o arquivo de exemplo de variáveis de ambiente para o arquivo efetivo sem alterar nenhum valor, e executa o comando de subida documentado no README
- **THEN** os três serviços ficam disponíveis, o endpoint de saúde do backend responde `200`, e o frontend é servido com sucesso na porta documentada

#### Scenario: Nenhum serviço depende de ferramenta instalada no host

- **WHEN** o ambiente é levantado numa máquina que não tem Go, Node nem PostgreSQL instalados
- **THEN** a subida completa mesmo assim, porque todo build ocorre dentro dos containers

### Requirement: Backend só aceita tráfego depois que o banco está pronto

O backend SHALL aguardar o banco de dados estar aceitando conexões antes de reportar-se saudável, e SHALL falhar de forma explícita e com mensagem acionável caso o banco não fique disponível dentro de um limite de tempo configurável.

#### Scenario: Banco ainda inicializando

- **WHEN** o backend inicia enquanto o PostgreSQL ainda está em processo de inicialização
- **THEN** o backend aguarda, registra em log que está aguardando o banco, e passa a reportar-se saudável assim que a conexão é estabelecida

#### Scenario: Banco inalcançável

- **WHEN** o banco não fica disponível dentro do limite de tempo configurado
- **THEN** o backend encerra com código de saída diferente de zero e uma mensagem de log que nomeia o host de destino e a causa da falha

### Requirement: Configuração exclusivamente por variáveis de ambiente

Toda configuração de runtime SHALL vir de variáveis de ambiente. O repositório SHALL conter um arquivo de exemplo documentando cada variável reconhecida, e esse arquivo SHALL conter apenas valores de desenvolvimento — nunca segredos reais.

#### Scenario: Variável obrigatória ausente

- **WHEN** o backend inicia sem uma variável de ambiente obrigatória definida
- **THEN** ele encerra imediatamente com uma mensagem que nomeia a variável faltante, em vez de assumir um valor padrão silenciosamente

#### Scenario: Exemplo cobre toda variável lida

- **WHEN** o conjunto de variáveis lidas pelo código é comparado com o arquivo de exemplo
- **THEN** toda variável lida está documentada no exemplo, com descrição do que faz

#### Scenario: Arquivo efetivo nunca é versionado

- **WHEN** o arquivo efetivo de variáveis de ambiente existe no diretório de trabalho
- **THEN** ele é ignorado pelo controle de versão e não pode ser commitado por engano

### Requirement: Migrações versionadas e idempotentes

O schema do banco SHALL ser gerenciado por migrações versionadas e ordenadas, aplicadas automaticamente na subida do backend. A aplicação SHALL ser idempotente: rodar novamente sobre um banco já migrado não altera nada e não falha.

#### Scenario: Banco vazio

- **WHEN** o sistema sobe apontando para um banco sem nenhuma tabela
- **THEN** todas as migrações são aplicadas em ordem e o schema resultante fica completo

#### Scenario: Banco já migrado

- **WHEN** o sistema sobe novamente sobre um banco que já está na versão mais recente
- **THEN** nenhuma migração é reaplicada, nenhum dado é alterado, e a subida conclui normalmente

#### Scenario: Volume de dados preexistente

- **WHEN** uma migração nova é adicionada e o sistema sobe sobre um volume de banco que já continha dados de uma versão anterior do schema
- **THEN** apenas a migração nova é aplicada, e os dados preexistentes são preservados

#### Scenario: Migração falha

- **WHEN** uma migração falha durante a aplicação
- **THEN** o backend não passa a servir tráfego, o erro é registrado identificando qual migração falhou, e a versão do schema não avança

### Requirement: Migrações aplicadas são imutáveis

Uma migração já publicada no repositório SHALL ser tratada como imutável. Alterações de schema SHALL ser expressas como uma migração nova, nunca por edição de uma existente.

#### Scenario: Necessidade de corrigir schema já migrado

- **WHEN** um erro é identificado num schema definido por uma migração já publicada
- **THEN** a correção entra como uma migração subsequente, e o arquivo original permanece inalterado
