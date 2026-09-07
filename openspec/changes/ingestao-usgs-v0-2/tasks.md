## 1. Registro da fonte e condições de uso

- [x] 1.1 Documentar em `docs/DATA_SOURCES.md` a seção do USGS Earthquake Hazards Program — licença (domínio público dos EUA, obra de agência federal), texto de atribuição exigido, URL dos termos, cadência de publicação da fonte e cadência de coleta pretendida; verificar por inspeção que cada campo tem origem citável, sem valor inventado
- [x] 1.2 Registrar em `docs/DATA_SOURCES.md` a decisão sobre o PVMBG com a evidência que a sustenta (`robots.txt` com `Disallow: /`, ausência de API e de termos, aviso de direitos reservados) e o que exatamente destravaria a fonte; verificar que o documento diz o que fazer para desbloquear, não apenas que está bloqueado
- [x] 1.3 Migração adicionando a `data_sources` as colunas de cadência de coleta, política de coleta declarada pela fonte e motivo de bloqueio; verificar com teste que fonte com política de bloqueio não pode ser habilitada, mesmo com licença preenchida
- [x] 1.4 Migração de dados preenchendo licença, atribuição, termos e cadências do USGS e habilitando a fonte, e gravando o motivo do bloqueio do PVMBG; verificar com teste que o USGS fica habilitado, que o PVMBG permanece desabilitado com motivo, e que nenhuma outra fonte foi habilitada de carona

## 2. Schema da ingestão

- [x] 2.1 Migração acrescentando a `earthquakes` o status do evento na fonte, o tipo de magnitude e os indicadores de qualidade da solução (`rms`, `gap`, `nst`, `dmin`); verificar com teste que a migração aplica sobre a tabela existente e que os índices temporais da `006` continuam válidos
- [x] 2.2 Migração acrescentando a `earthquakes` o estado de qualidade `NOT NULL` **sem `DEFAULT`**, o motivo, a versão do parser, a versão da fonte e o dado cru; verificar com testes que `INSERT` omitindo o estado de qualidade falha no banco, e que estado fora do conjunto permitido é rejeitado
- [x] 2.3 Migração criando a tabela de execuções de ingestão — fonte, início, fim, janela consultada, resultado, contagens de inseridos/atualizados/rejeitados e mensagem de erro — com índice por fonte e instante; verificar com teste que duas execuções da mesma fonte coexistem e que a mais recente é recuperável por consulta indexada
- [x] 2.4 Verificação de integração das migrações: aplicar da base vazia e sobre um banco já migrado da V0.1, confirmando que subir duas vezes não reaplica e que a `earthquakes` preexistente sobrevive com os dados que tiver

## 3. Cliente HTTP da fonte

- [x] 3.1 Implementar em `internal/usgs` o cliente do FDSN Event API com tempo limite, limite de tentativas, espera crescente entre elas e `User-Agent` que identifica o projeto; verificar com testes contra servidor HTTP local que simula timeout, `500`, corpo truncado e sucesso, afirmando o número de tentativas em cada caso
- [ ] 3.2 Capturar da API real e versionar em `testdata` as respostas de referência, incluindo **o mesmo evento antes e depois de revisão pelo USGS**, com README marcando-as como amostra datada e não como catálogo; verificar que a suíte inteira roda sem acesso à rede
- [x] 3.3 Implementar a montagem da consulta por janela (`starttime`/`endtime`) e por atualização (`updatedafter`), com paginação por `limit`/`offset` respeitando o teto de 20.000 eventos por requisição; verificar com teste que uma janela que excede o teto é percorrida em páginas sem repetir nem omitir evento

## 4. Parser, validação e normalização

- [x] 4.1 Implementar o parser do GeoJSON do USGS extraindo identificador, instante, coordenadas, profundidade, magnitude, tipo de magnitude, status e indicadores da solução; verificar com testes sobre a fixture real e sobre resposta deliberadamente malformada, afirmando que a malformada falha nomeando a etapa de parse
- [x] 4.2 Fazer o parser distinguir campo ausente de valor zero para magnitude e profundidade; verificar com teste sobre evento real sem magnitude que o valor persistido é ausente, e não zero
- [ ] 4.3 Implementar a validação de registro individual — coordenada em faixa válida, instante presente e interpretável, identificador não vazio — rejeitando o registro isolado e prosseguindo com os demais; verificar com teste que um lote com um registro inválido persiste os válidos e relata a contagem de rejeitados
- [x] 4.4 Implementar a normalização: timestamps em UTC, coordenadas em SRID 4326, profundidade em quilômetros com unidade explícita, identificador da fonte preservado sem reescrita; verificar com teste que compara o registro normalizado com o dado cru correspondente campo a campo
- [x] 4.5 Gravar a versão do parser em cada registro e fazê-la mudar quando a extração mudar; verificar com teste que registros produzidos por versões diferentes são distinguíveis por consulta

## 5. Motor de qualidade

- [x] 5.1 Implementar em `internal/dataquality` os estados (`valid`, `suspect`, `duplicate`, `outlier`, `corrupted`, `delayed`) sem zero-value útil, de modo que esquecer de avaliar seja erro e não `valid` implícito; verificar com teste que persistir sem estado avaliado é rejeitado
- [x] 5.2 Implementar as verificações nomeadas de valor impossível, timestamp inválido, duplicata e chegada tardia, cada uma gravando no motivo o nome estável da regra que a produziu; verificar com testes um caso por regra, afirmando o estado **e** o nome da regra no motivo
- [ ] 5.3 Garantir que registro que falhou verificação é persistido marcado, nunca descartado; verificar com teste que um lote com registro implausível persiste esse registro com estado não-`valid` e o mantém recuperável
- [ ] 5.4 Verificar com teste que reavaliar não altera versão anterior: revisão que corrige valor implausível gera versão `valid` e a consulta as-of anterior à revisão continua devolvendo `suspect`

## 6. Persistência e leitura temporal

- [x] 6.1 Implementar `internal/earthquake` como único lugar autorizado a escrever predicado temporal sobre `earthquakes`, com leitura corrente e as-of, `Provenance` sem zero-value útil e as-of futuro rejeitado; verificar com testes espelhando os de `internal/observation`, inclusive o de que `ingested_at` informado pelo chamador é ignorado
- [x] 6.2 Implementar a escrita com deduplicação por comparação de conteúdo normalizado, não por confiança no `updated` da fonte; verificar com testes que reingerir a mesma resposta relata zero inserções e zero atualizações, e que payload idêntico com `updated` novo **não** cria versão
- [ ] 6.3 Verificar com teste, usando a fixture do mesmo evento antes e depois da revisão real do USGS, que a revisão cria versão nova, que a leitura corrente devolve só a nova, e que a as-of anterior devolve a magnitude antiga — o teste que a V0.1 não teve como escrever
- [x] 6.4 Escrever teste de data leakage sobre sismos, percorrendo instantes as-of e afirmando que nenhum resultado tem `ingested_at` posterior ao instante consultado; verificar por sabotagem deliberada do predicado que o teste fica vermelho — a sabotagem tem de compilar
- [x] 6.5 Implementar a consulta por proximidade e por magnitude mínima sobre PostGIS, com distância em cada item; verificar com teste de integração sobre coordenadas conhecidas e com teste de que latitude fora de faixa é rejeitada

## 7. Execuções, cobertura e lacunas

- [ ] 7.1 Implementar o registro de execução de ingestão cobrindo início, fim, janela, resultado, contagens e erro, gravado inclusive quando a execução falha; verificar com teste que uma execução que falha no meio deixa registro de falha e **não** sobrescreve o da execução anterior
- [ ] 7.2 Implementar a derivação da cobertura de ingestão a partir das execuções, respondendo se uma janela foi coberta, parcialmente coberta ou não coberta; verificar com teste que janela sem execução bem-sucedida é relatada como não coberta mesmo quando a tabela de sismos está vazia
- [ ] 7.3 Implementar a identificação de lacunas — intervalo entre execuções bem-sucedidas maior que a cadência de coleta — com início e fim; verificar com teste que uma sequência com falha no meio produz exatamente a lacuna esperada
- [ ] 7.4 Implementar a consulta de estado operacional por fonte (resultado e instante da última execução, última coleta bem-sucedida, há quanto tempo sem sucesso); verificar com testes que fonte habilitada nunca coletada é distinguível de fonte cuja última coleta falhou

## 8. Execução manual e automática

- [ ] 8.1 Expor o subcomando `ingest` com janela explícita e modo de backfill parametrizável, padrão de 90 dias; verificar executando contra a API real e conferindo a contagem ingerida contra o que a API relata para a mesma janela, com a saída colada no relatório da tarefa
- [ ] 8.2 Implementar o agendador in-process disparando o ciclo em intervalo configurável, subindo **depois** do listener HTTP; verificar com teste que o serviço atende requisições normalmente com a fonte inacessível e que a falha fica registrada como execução
- [ ] 8.3 Garantir a não-sobreposição por advisory lock no Postgres, e não por trava em memória; verificar com teste de integração que dois ciclos concorrentes para a mesma fonte resultam em um executando e outro registrando que não iniciou
- [ ] 8.4 Implementar a âncora incremental na última execução bem-sucedida com sobreposição de segurança; verificar com teste que um evento revisado pela fonte no intervalo entre dois ciclos consecutivos não se perde
- [ ] 8.5 Verificar que a ingestão é retomável: interromper uma execução no meio e rodar de novo deixa o banco consistente, sem versão duplicada e sem evento faltando na janela

## 9. API

- [ ] 9.1 Implementar `GET /api/v1/earthquakes` sob o envelope existente, com filtros de janela, as-of, proximidade, magnitude mínima, proveniência e qualidade; verificar com testes que percorrer a coleção inteira não repete nem omite, que janela invertida retorna `400` nomeando o parâmetro, e que proximidade meio-especificada retorna `400`
- [ ] 9.2 Expor o estado de qualidade e o motivo em cada registro servido, com filtro por qualidade; verificar com testes que o filtro por `valid` não devolve registro de outro estado e que a composição de qualidade de um conjunto misto é observável na resposta
- [ ] 9.3 Declarar a cobertura de ingestão nas respostas com janela de tempo, de modo que coleção vazia com cobertura completa seja distinguível de coleção vazia por falta de coleta; verificar com teste que exercita os dois casos e afirma que as respostas diferem
- [ ] 9.4 Expor o estado operacional das fontes na API, sem vazar SQL, caminho de arquivo, credencial nem rastro de pilha; verificar com teste que a causa de uma falha de ingestão aparece descrita e higienizada
- [ ] 9.5 Permitir chegar ao dado cru e à versão do parser de um registro ingerido; verificar com teste que o dado cru devolvido permite rederivar os campos normalizados daquele registro

## 10. Testes, documentação e verificação da fase

- [ ] 10.1 Garantir que a suíte inteira roda sem acesso à rede e que `go test -short ./...` continua rodando sem Docker; verificar rodando a suíte completa com a rede desligada
- [ ] 10.2 Atualizar o `README.md` com o subcomando de ingestão, a configuração do agendador e as rotas novas; verificar seguindo o próprio README e exercitando as rotas documentadas contra o serviço rodando
- [ ] 10.3 Documentar em `docs/` a anatomia de um adaptador de fonte — as etapas, o que cada execução registra, e como escrever o próximo conector — referenciando o teste de revisão real como exemplo executável; verificar que o documento é suficiente para alguém começar o segundo conector sem ler o código do primeiro
- [ ] 10.4 Verificação final da fase contra a §88: confirmar implementação, testes, tratamento de erro, documentação, dados de exemplo, validação, logs e README atualizado, declarando explicitamente qualquer lacuna em vez de reclassificá-la como pronta
