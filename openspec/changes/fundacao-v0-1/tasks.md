## 1. Ambiente sobe de clone limpo

- [x] 1.1 Adicionar `.gitignore` cobrindo `.env`, `node_modules/`, binários de build e artefatos de teste; verificar com `git status --ignored` que `.env` aparece como ignorado
- [x] 1.2 Criar `.env.example` documentando cada variável (`DATABASE_URL`, `HTTP_ADDR`, `DB_CONNECT_TIMEOUT`, `LOG_LEVEL`) com valores de desenvolvimento e nenhum segredo; verificar por inspeção que toda variável lida pelo código está listada
- [x] 1.3 Gerar `backend/go.sum` e remover de `go.mod` qualquer dependência não importada; verificar que `go mod verify` e `go build ./...` passam
- [x] 1.4 Criar `frontend/tsconfig.json` e `frontend/vite.config.ts` com o plugin React aplicado; verificar que `npm run build` conclui sem erro
- [x] 1.5 Corrigir `frontend/index.html` para documento HTML completo com `<!doctype>`, `<head>` e `<title>`; verificar que o build não emite aviso de HTML inválido
- [x] 1.6 Criar `frontend/Dockerfile` (build multi-stage, servindo em modo dev na porta documentada); verificar que `docker compose build frontend` conclui
- [x] 1.7 Adicionar `healthcheck` ao serviço `db` e `depends_on: condition: service_healthy` no backend do `docker-compose.yml`; verificar que o backend não inicia antes do banco aceitar conexões
- [x] 1.8 Trocar o placeholder `SEU_USUARIO` no caminho do módulo em `go.mod` pelo caminho real do repositório e ajustar imports; verificar que `go build ./...` passa
- [x] 1.9 Verificação de integração: a partir de um clone limpo em diretório temporário, `cp .env.example .env && docker compose up --build` deixa `/health` respondendo `200` e o frontend servido, sem passo manual adicional

## 2. Configuração e conexão com o banco

- [x] 2.1 Implementar `internal/config` lendo e validando variáveis de ambiente, encerrando com código diferente de zero e mensagem que nomeia a variável faltante; verificar com teste de unidade para variável ausente, valor inválido e caso feliz
- [x] 2.2 Implementar `internal/db` com pool `pgx` e espera pelo banco até `DB_CONNECT_TIMEOUT`, registrando em log a espera; verificar com teste que simula banco indisponível e confirma saída não-zero com o host nomeado na mensagem
- [x] 2.3 Adicionar `goose` como biblioteca com migrações embutidas por `embed.FS`, aplicadas antes de abrir o listener HTTP, com advisory lock; verificar que subir duas vezes seguidas não reaplica migração e que falha de migração impede o servidor de servir tráfego

## 3. Schema bitemporal

- [x] 3.1 Escrever migração `003` criando a tabela de controle do goose e reconciliando o schema herdado do `docker-entrypoint-initdb.d` com `IF EXISTS`/`IF NOT EXISTS`; verificar que ela aplica com sucesso tanto sobre volume novo quanto sobre volume com o schema das migrações `001`/`002`
- [x] 3.2 Escrever migração recriando `observations` na forma bitemporal — `observed_at`, `ingested_at NOT NULL DEFAULT now()`, `is_synthetic BOOLEAN NOT NULL` sem default, `source_id` com chave estrangeira para `data_sources`, unicidade em `(chave natural, ingested_at)`; verificar com testes que `INSERT` omitindo `is_synthetic` falha, que `INSERT` sem `observed_at` falha, e que `source_id` inexistente é rejeitado
- [x] 3.3 Escrever migração recriando `earthquakes` na forma bitemporal com a mesma disciplina, usando o identificador da fonte como chave natural; verificar com teste que duas versões do mesmo sismo coexistem com `ingested_at` distintos
- [x] 3.4 Criar índices em `(chave natural, ingested_at DESC)` e em `observed_at` para ambas as tabelas; verificar com teste que inspeciona `EXPLAIN` de uma consulta as-of por janela e confirma uso de índice, sem varredura sequencial
- [x] 3.5 Adicionar `updated_at` e trigger de auditoria em `data_sources`, mais colunas `license`, `attribution`, `terms_url` e `update_cadence`; verificar com teste que habilitar fonte com `license` vazia é rejeitado
- [x] 3.6 Adicionar restrição impedindo remoção de fonte com dados associados; verificar com teste que `DELETE` de fonte referenciada falha e que desabilitar a mesma fonte funciona
- [x] 3.7 Adicionar `source_id`, `source_ref` e `absent_from_source_at` a `volcanoes`, com unicidade em `(source_id, source_ref)` e restrição de faixa válida em latitude e longitude; verificar com teste que latitude 91 é rejeitada e que o mesmo `source_ref` não pode ser inserido duas vezes para a mesma fonte

## 4. Leitura temporal

- [ ] 4.1 Implementar em `internal/observation` a leitura corrente (versão de maior `ingested_at` por chave natural) e a leitura as-of restrita a `ingested_at <= T`; verificar com testes que registro ingerido após T é invisível na consulta as-of, que a leitura corrente não duplica por versão, e que a mesma consulta as-of repetida retorna conjunto idêntico
- [ ] 4.2 Rejeitar consulta as-of com instante futuro; verificar com teste que retorna erro em vez de estado atual
- [ ] 4.3 Implementar filtro por proveniência (apenas reais, apenas sintéticos, ambos); verificar com teste que o filtro por reais não retorna nenhum sintético
- [ ] 4.4 Garantir que `ingested_at` fornecido pelo chamador é ignorado na escrita; verificar com teste que persiste informando `ingested_at` no passado e confirma que o valor gravado é o instante da persistência
- [ ] 4.5 Escrever teste de data leakage que percorre uma sequência de instantes as-of e afirma que nenhum resultado contém registro com `ingested_at` posterior ao instante consultado

## 5. Registro de fontes e catálogo de vulcões

- [ ] 5.1 Ler os termos de uso do Smithsonian GVP e decidir se o snapshot bruto pode ser redistribuído no repositório ou apenas referenciado por checksum e instrução de download; registrar a decisão em `docs/DATA_SOURCES.md` e resolver a Open Question do design
- [ ] 5.2 Migração de dados atualizando as fontes já cadastradas com licença, atribuição e cadência, e desabilitando toda fonte cuja licença ainda não foi determinada; verificar com teste que nenhuma fonte fica habilitada sem licença
- [ ] 5.3 Obter o snapshot da Holocene Volcano List e commitá-lo em `backend/data/gvp/` conforme a decisão de 5.1, junto de um manifesto com versão da base, DOI, data do download e SHA-256; verificar que o checksum registrado confere com o arquivo
- [ ] 5.4 Implementar `internal/gvp` parseando o snapshot e validando a estrutura esperada, falhando explicitamente diante de formato inesperado; verificar com testes sobre uma amostra reduzida do arquivo real e sobre um arquivo deliberadamente malformado
- [ ] 5.5 Implementar o importador com semântica de inserir/atualizar/marcar-ausente, sem apagar; verificar com testes que a segunda execução sem mudança na fonte relata zero inserções e zero atualizações, que atributo alterado gera atualização, e que vulcão sumido da fonte é marcado e não removido
- [ ] 5.6 Fazer o importador rejeitar registros com coordenada inválida, registrando o vulcão em log e prosseguindo com os demais; verificar com teste que uma amostra com um registro inválido importa os válidos e relata a contagem de rejeitados
- [ ] 5.7 Expor o importador como subcomando do binário e documentar seu uso no README; verificar executando-o contra o banco local e conferindo a contagem de vulcões importados
- [ ] 5.8 Verificação de integração: reexecutar o importador duas vezes seguidas e confirmar que os identificadores dos vulcões permanecem estáveis e que observações associadas continuam íntegras

## 6. API sobre o banco

- [ ] 6.1 Implementar em `internal/httpapi` o envelope de resposta (`data`, `meta`, `attribution`) e o formato único de erro com código estável e identificador de correlação; verificar com testes que erro `400` nomeia o parâmetro rejeitado e que `500` não expõe SQL, caminho de arquivo nem rastro de pilha
- [ ] 6.2 Preencher `meta.disclaimer` na camada de serialização para toda resposta com valor derivado; verificar com teste que a ressalva permanece mesmo quando a requisição tenta suprimi-la por parâmetro ou cabeçalho
- [ ] 6.3 Implementar paginação por keyset com cursor opaco e ordenação total incluindo `id`; verificar com testes que percorrer a coleção inteira não repete nem omite item, que limite acima do máximo retorna `400`, e que coleção vazia retorna `200`
- [ ] 6.4 Implementar `GET /api/v1/volcanoes` com filtro por proximidade usando PostGIS, retornando distância em cada item e ordenando por distância crescente; verificar com teste de integração sobre coordenadas conhecidas e com teste de que latitude fora de faixa retorna `400`
- [ ] 6.5 Reescrever `GET /api/v1/events` para ler do banco com filtro por janela temporal, instante as-of e proveniência, removendo o evento demo hardcoded; verificar com teste que janela invertida retorna `400` e que a ausência de dado real retorna coleção vazia em vez de dado sintético
- [ ] 6.6 Fazer `/health` refletir conectividade com o banco e versão de schema esperada; verificar com testes que banco indisponível e schema desatualizado impedem `200`
- [ ] 6.7 Retornar `404` para caminhos da API sem prefixo de versão; verificar com teste
- [ ] 6.8 Implementar middleware de log estruturado com método, caminho, status, duração e identificador de correlação, redigindo valores sensíveis; verificar com testes que o identificador da resposta `500` aparece no log e que um valor sensível em parâmetro sai redigido
- [ ] 6.9 Expor a atribuição da fonte nas respostas que servem dado externo; verificar com teste que um registro originado do GVP permite chegar ao texto de atribuição exigido

## 7. Testes, CI e documentação

- [x] 7.1 Configurar `testcontainers-go` com PostGIS efêmero e a convenção de `go test -short` pular integração; verificar que `go test -short ./...` roda sem Docker e que `go test ./...` sobe o container
- [ ] 7.2 Criar workflow do GitHub Actions rodando build, `go vet`, testes Go completos e build do frontend; verificar que o workflow passa verde na primeira execução
- [ ] 7.3 Atualizar `README.md` com instruções reais de subida, execução do importador, execução dos testes e a ressalva científica; verificar seguindo o próprio README numa máquina limpa
- [ ] 7.4 Atualizar `docs/DATA_SOURCES.md` com licença, atribuição, cadência e versão de snapshot por fonte; verificar que toda fonte habilitada no banco tem entrada correspondente no documento
- [ ] 7.5 Documentar em `docs/` o modelo bitemporal e como escrever uma consulta as-of correta, referenciando o teste de data leakage como exemplo executável
- [ ] 7.6 Verificação final da fase contra a §88 da master spec: confirmar que existem implementação, testes, tratamento de erro, documentação, dados de exemplo, validação, logs e README atualizado
