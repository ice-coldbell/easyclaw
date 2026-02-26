
import { createRoot } from "react-dom/client";
import App from "./app/App.tsx";
import { I18nProvider } from "./app/i18n";
import { WalletProvider } from "./app/lib/wallet";
import "./styles/index.css";

createRoot(document.getElementById("root")!).render(
  <WalletProvider>
    <I18nProvider>
      <App />
    </I18nProvider>
  </WalletProvider>,
);
  
