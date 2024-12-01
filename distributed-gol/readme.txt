SETUP -------------------------------------------------------------------------------------------------------

in worker dir -             ./start_workers.sh <number_of_workers>
in engine dir -             go run . -startPort=<start> -endPort=<end>
in distributed-gol dir -    go run .

PROTOCOLS USED ----------------------------------------------------------------------------------------------

RPC (Remote Procedure Calls) uses TCP (Transmission Control Protocol)

DESIGN PATTERNS USED ----------------------------------------------------------------------------------------

Master-Worker Pattern : Broker acts as a 'master', delegating tasks to multiple worker nodes.
Singleton Pattern : Broker can only be instantiated once as address already in use.

PGM USAGE ---------------------------------------------------------------------------------------------------

Portable GrayMap files, simple grayscale image formats.
Specifically designed for grayscale images, making it lightweight compared to color formats like JPEG or PNG.
Store pixel intensity values from 0 (black) to 255 (white).
Plain text (P2) or binary (P5) formats.
Ideal for processing raw image data.
