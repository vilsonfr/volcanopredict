import {
  Ion,
  UrlTemplateImageryProvider,
  EllipsoidTerrainProvider,
  createWorldTerrainAsync,
  type ImageryProvider,
  type TerrainProvider,
} from "cesium";

/**
 * Imagery and terrain sources for the globe.
 *
 * The project registers every external data source with its license and
 * attribution (see openspec/.../registro-de-fontes). Basemap tiles are no
 * exception: each provider below carries the credit string its terms require,
 * and Cesium renders it in the on-screen credit bar.
 */

/** Esri World Imagery. No API key required. Attribution is mandatory. */
const ESRI_WORLD_IMAGERY_URL =
  "https://services.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}";

const ESRI_CREDIT =
  "Imagery © Esri, Maxar, Earthstar Geographics, and the GIS User Community";

export type ImagerySetup = {
  imageryProvider: ImageryProvider;
  terrainProvider: TerrainProvider;
  /** True when real elevation data is active, rather than a smooth ellipsoid. */
  hasRealTerrain: boolean;
};

/**
 * Builds the globe's imagery and terrain.
 *
 * Without a Cesium Ion token the globe still shows real satellite imagery, but
 * the surface is a smooth ellipsoid — volcanoes look flat. Supplying a free Ion
 * token in VITE_CESIUM_ION_TOKEN enables Cesium World Terrain, which is what
 * makes relief (and therefore volcano cones) actually visible.
 */
export async function buildImagery(): Promise<ImagerySetup> {
  const imageryProvider = new UrlTemplateImageryProvider({
    url: ESRI_WORLD_IMAGERY_URL,
    maximumLevel: 19,
    credit: ESRI_CREDIT,
  });

  const ionToken = import.meta.env.VITE_CESIUM_ION_TOKEN?.trim();
  if (!ionToken) {
    return {
      imageryProvider,
      terrainProvider: new EllipsoidTerrainProvider(),
      hasRealTerrain: false,
    };
  }

  Ion.defaultAccessToken = ionToken;
  try {
    return {
      imageryProvider,
      terrainProvider: await createWorldTerrainAsync(),
      hasRealTerrain: true,
    };
  } catch (err) {
    // A bad or expired token must not blank the globe: degrade to the
    // ellipsoid and say so, rather than failing the whole render.
    console.warn(
      "Cesium Ion terrain unavailable, falling back to smooth ellipsoid:",
      err,
    );
    return {
      imageryProvider,
      terrainProvider: new EllipsoidTerrainProvider(),
      hasRealTerrain: false,
    };
  }
}
