/**
 * CSS Modules type declaration for .module.css files.
 */
declare module '*.module.css' {
  const classes: Readonly<Record<string, string>>
  export default classes
}
