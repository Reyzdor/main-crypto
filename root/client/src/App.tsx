import { useState } from "react"
import Snowfall from "react-snowfall";

function App() {
  const [color] = useState({
    dark: "#111111",
    accent: "#222222"
  });

  const [colorText] = useState("#FFFFFF")

  return (
    <>
      <div style={{background: `linear-gradient(120deg, ${color.dark}, ${color.accent})`, height: '100vh', padding: '20px'}}>
        <h1 style={{ color: colorText, textAlign: "center"}}>Soon</h1>
        <Snowfall/>
      </div>
    </>
      
  )
}

export default App
