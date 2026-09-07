import { StrictMode, useCallback, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { Globe, type GlobeControls } from "./globe/Globe";
import { SearchPanel } from "./ui/SearchPanel";
import type { Volcano } from "./api/client";
import { AboutModal } from "./ui/AboutModal";
import "./style.css";

function App() {
  const [hasRealTerrain, setHasRealTerrain] = useState<boolean | null>(null);
  const [aboutOpen, setAboutOpen] = useState(false);
  const [volcanoCount, setVolcanoCount] = useState<number | null>(null);
  const [volcanoError, setVolcanoError] = useState<string | null>(null);
  const [volcanoesVisible, setVolcanoesVisible] = useState(true);
  const controlsRef = useRef<GlobeControls | null>(null);
  const onReady = useCallback((c: GlobeControls) => {
    controlsRef.current = c;
  }, []);
  const focusVolcano = useCallback((v: Volcano) => {
    controlsRef.current?.focus(v);
  }, []);
  const toggleVolcanoes = useCallback((visible: boolean) => {
    setVolcanoesVisible(visible);
    controlsRef.current?.setVolcanoesVisible(visible);
  }, []);
  const onVolcanoesLoaded = useCallback((count: number, error?: string) => {
    setVolcanoCount(count);
    setVolcanoError(error ?? null);
  }, []);
  const onTerrainResolved = useCallback(
    (v: boolean) => setHasRealTerrain(v),
    [],
  );

  return (
    <main>
      <header>
        <h1>🌋 VolcanoPredict</h1>
        <div className="header__right">
          <span>pesquisa aberta</span>
          <button
            type="button"
            className="about__trigger"
            onClick={() => setAboutOpen(true)}
          >
            Sobre
          </button>
        </div>
      </header>

      <section className="stage">
        <Globe
          onTerrainResolved={onTerrainResolved}
          onVolcanoesLoaded={onVolcanoesLoaded}
          onReady={onReady}
        />

        <aside className="panel">
          <SearchPanel onSelect={focusVolcano} />

          <h2>Camadas</h2>
          <label>
            <input
              type="checkbox"
              checked={volcanoesVisible}
              onChange={(e) => toggleVolcanoes(e.target.checked)}
            />{" "}
            Vulcões
            <em>
              {volcanoError
                ? "indisponível"
                : volcanoCount === null
                  ? "carregando…"
                  : `${volcanoCount.toLocaleString("pt-BR")}`}
            </em>
          </label>
          <label>
            <input type="checkbox" disabled /> Terremotos <em>em breve</em>
          </label>
          <label>
            <input type="checkbox" disabled /> Placas tectônicas <em>em breve</em>
          </label>
          <label>
            <input type="checkbox" disabled /> Vento / cinzas <em>futuro</em>
          </label>
          <label>
            <input type="checkbox" disabled /> Tsunami <em>futuro</em>
          </label>
          <label>
            <input type="checkbox" disabled /> Atividade solar <em>futuro</em>
          </label>

          {volcanoesVisible && volcanoCount !== null && !volcanoError && (
            <div className="legend">
              <h2>Evidência de atividade</h2>
              <ul>
                <li>
                  <i style={{ background: "#ff5a3c" }} /> Erupção observada
                </li>
                <li>
                  <i style={{ background: "#ffa63c" }} /> Erupção datada
                </li>
                <li>
                  <i style={{ background: "#ffd93c" }} /> Evidência credível
                </li>
                <li>
                  <i style={{ background: "#9fb4c7" }} /> Evidência incerta
                </li>
              </ul>
              <p>
                Classificação declarada pela própria fonte. Não é um risco
                calculado por este sistema.
              </p>
            </div>
          )}

          {volcanoError && (
            <p className="hint hint--error">
              Não foi possível carregar o catálogo pela API: {volcanoError}
            </p>
          )}
          {hasRealTerrain === true && (
            <p className="hint hint--ok">Relevo do terreno ativo.</p>
          )}
        </aside>
      </section>

      <footer>
        Score experimental de risco ≠ previsão determinística. Sempre consulte
        autoridades oficiais.
      </footer>

      <AboutModal open={aboutOpen} onClose={() => setAboutOpen(false)} />
    </main>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
