import React from "react";
import { createRoot } from "react-dom/client";
import "./style.css";

function App() {
  return <main>
    <header><h1>🌋 VolcanoPredict</h1><span>V0.1 • pesquisa aberta</span></header>
    <section className="globe">
      <div className="earth">🌎</div>
      <div className="panel">
        <h2>Mapa científico</h2>
        <label><input type="checkbox" defaultChecked/> Vulcões</label>
        <label><input type="checkbox" defaultChecked/> Terremotos</label>
        <label><input type="checkbox"/> Placas tectônicas</label>
        <label><input type="checkbox"/> Vento / cinzas</label>
        <label><input type="checkbox"/> Tsunami</label>
        <label><input type="checkbox"/> Atividade solar</label>
      </div>
    </section>
    <footer>Score experimental de risco ≠ previsão determinística. Sempre consulte autoridades oficiais.</footer>
  </main>
}
createRoot(document.getElementById("root")!).render(<App />);
