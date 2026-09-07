-- +goose Up
-- Task 1.4 (registro-de-fontes spec, V0.2). Habilita o USGS Earthquake
-- Hazards Program como primeira fonte de ingestão contínua, e registra o
-- bloqueio do PVMBG com a evidência que o sustenta.
--
-- Os valores abaixo vieram de leitura dos termos, não de suposição; a
-- pesquisa está em docs/DATA_SOURCES.md. Espelha a migração 010, que fez
-- o mesmo para o GVP.

UPDATE data_sources
SET
    license = 'public-domain-us-govt-work-attribution-required',
    attribution = 'U.S. Geological Survey',
    terms_url = 'https://www.usgs.gov/information-policies-and-instructions/copyrights-and-credits',
    -- Cadência da FONTE: contínua, e — o que importa para o modelo
    -- bitemporal — com revisão de eventos já publicados, de `automatic`
    -- para `reviewed`, incluindo substituição de magnitude preliminar.
    update_cadence = 'continuous; events are revised after publication (automatic -> reviewed, magnitudes superseded)',
    -- Cadência NOSSA: periódica e configurável, ancorada na última
    -- execução bem-sucedida.
    collection_cadence = 'periodic, configurable; incremental by updatedafter anchored on the last successful run',
    enabled = true
WHERE name = 'USGS Earthquake Hazards Program';

-- O PVMBG não é bloqueado por licença indeterminada, e sim por proibição
-- explícita de coleta. Registrar isso na linha impede que uma futura
-- migração de licenciamento o habilite por engano — a restrição da 011
-- recusa.
UPDATE data_sources
SET
    collection_policy = 'robots.txt: "User-agent: * / Disallow: /" — proíbe coleta automatizada de todo o site',
    blocked_reason = 'A fonte proíbe coleta automatizada (robots.txt Disallow: /), não publica API documentada nem termos de uso, e declara direitos reservados. Desbloquear exige permissão escrita do PVMBG/Badan Geologi ou publicação de API oficial com termos. Ver docs/DATA_SOURCES.md.',
    enabled = false
WHERE name = 'PVMBG';

-- +goose Down
UPDATE data_sources
SET
    license = NULL,
    attribution = NULL,
    terms_url = NULL,
    update_cadence = NULL,
    collection_cadence = NULL,
    enabled = false
WHERE name = 'USGS Earthquake Hazards Program';

UPDATE data_sources
SET
    collection_policy = NULL,
    blocked_reason = NULL
WHERE name = 'PVMBG';
